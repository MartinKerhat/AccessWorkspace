package app

import (
	"context"
	"sync"

	"access-workspace/backend/internal/api"
	"access-workspace/backend/internal/appregistrations"
	"access-workspace/backend/internal/artifacts"
	"access-workspace/backend/internal/audit"
	"access-workspace/backend/internal/auth"
	"access-workspace/backend/internal/db"
	"access-workspace/backend/internal/keyvault"
	"access-workspace/backend/internal/notifications"
	"access-workspace/backend/internal/resources"
	"access-workspace/backend/internal/seed"
	"github.com/jackc/pgx/v5/pgxpool"
)

type App struct {
	db               *pgxpool.Pool
	handler          *api.Server
	backgroundCancel context.CancelFunc
	backgroundWG     sync.WaitGroup
}

type rdpSigningProvider struct {
	store *AdminConfigStore
}

type adminConfigProvider struct {
	*AdminConfigStore
}

func (p rdpSigningProvider) GetRDPSigningRuntime(ctx context.Context) (resources.RDPSigningRuntimeConfig, error) {
	config, err := p.store.GetRDPSigningRuntime(ctx)
	if err != nil {
		return resources.RDPSigningRuntimeConfig{}, err
	}
	return resources.RDPSigningRuntimeConfig{
		Enabled:               config.Enabled,
		CertificateConfigured: config.CertificateConfigured,
		Subject:               config.Subject,
		ThumbprintSHA256:      config.ThumbprintSHA256,
		PFXBase64:             config.PFXBase64,
		PFXPassword:           config.PFXPassword,
		LeafCertBase64:        config.LeafCertBase64,
		RootCertBase64:        config.RootCertBase64,
	}, nil
}

func (p adminConfigProvider) GetRDPSigningPublic(ctx context.Context) (api.RDPSigningPublicConfig, error) {
	config, err := p.AdminConfigStore.GetRDPSigningPublic(ctx)
	if err != nil {
		return api.RDPSigningPublicConfig{}, err
	}
	return api.RDPSigningPublicConfig{
		Enabled:               config.Enabled,
		CertificateConfigured: config.CertificateConfigured,
		Subject:               config.Subject,
		ThumbprintSHA256:      config.ThumbprintSHA256,
		LeafCertBase64:        config.LeafCertBase64,
		RootCertBase64:        config.RootCertBase64,
	}, nil
}

func New(cfg Config) (*App, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	ctx := context.Background()

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}

	if cfg.ResetDBOnStart {
		if err := db.ResetPublicSchema(ctx, pool); err != nil {
			pool.Close()
			return nil, err
		}
	}

	if cfg.AutoMigrate {
		if err := db.RunMigrations(ctx, pool); err != nil {
			pool.Close()
			return nil, err
		}
	}

	if cfg.SeedOnStart {
		if err := seed.Run(ctx, pool); err != nil {
			pool.Close()
			return nil, err
		}
	}

	if err := bootstrapAdmin(ctx, pool, cfg); err != nil {
		pool.Close()
		return nil, err
	}

	auditRepo := audit.NewRepository(pool)
	authRepo := auth.NewRepository(pool)
	resourceRepo := resources.NewRepository(pool)
	notificationRepo := notifications.NewRepository(pool)
	secretCipher := resources.NewSecretCipher(cfg.SecretEncryptionKey)
	adminStore := NewAdminConfigStore(pool, cfg, secretCipher)
	if err := adminStore.EncryptPlaintextSecretSettings(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	if err := authRepo.HashPlaintextSessionTokens(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	keyVaultService := keyvault.NewService(keyvault.SettingsProvider{
		Runtime: func(ctx context.Context) (keyvault.RuntimeConfig, error) {
			config, err := adminStore.GetEntraRuntime(ctx)
			if err != nil {
				return keyvault.RuntimeConfig{}, err
			}
			return keyvault.RuntimeConfig{
				Authority:    config.Authority,
				TenantID:     config.TenantID,
				ClientID:     config.ClientID,
				ClientSecret: config.ClientSecret,
				Configured:   config.TenantID != "" && config.ClientID != "" && config.ClientSecret != "",
			}, nil
		},
		Sources: func(ctx context.Context) ([]keyvault.Source, error) {
			sources, err := adminStore.ListKeyVaultSources(ctx)
			if err != nil {
				return nil, err
			}
			items := make([]keyvault.Source, 0, len(sources))
			for _, source := range sources {
				items = append(items, keyvault.Source{
					Name:     source.Name,
					VaultURL: source.VaultURL,
				})
			}
			return items, nil
		},
	})
	appRegistrationService := appregistrations.NewService(appregistrations.SettingsProvider{
		Runtime: func(ctx context.Context) (appregistrations.RuntimeConfig, error) {
			config, err := adminStore.GetEntraRuntime(ctx)
			if err != nil {
				return appregistrations.RuntimeConfig{}, err
			}
			return appregistrations.RuntimeConfig{
				Authority:    config.Authority,
				TenantID:     config.TenantID,
				ClientID:     config.ClientID,
				ClientSecret: config.ClientSecret,
				Configured:   config.TenantID != "" && config.ClientID != "" && config.ClientSecret != "",
			}, nil
		},
	})

	auditService := audit.NewService(auditRepo)
	authService := auth.NewService(authRepo, auth.Mode(cfg.AuthMode), cfg.EntraDirectRights)
	notificationService := notifications.NewService(notificationRepo, resourceRepo, authService, adminStore)
	resourceService := resources.NewService(resourceRepo, auditService, keyVaultService, appRegistrationService, notificationService, secretCipher, rdpSigningProvider{store: adminStore})

	artifactSource, err := artifacts.NewSource(artifacts.Config{
		Source:  cfg.ArtifactsSource,
		Dir:     cfg.ArtifactsDir,
		BaseURL: cfg.FrontendURL,
		BlobURL: cfg.ArtifactsBlobURL,
		BlobSAS: cfg.ArtifactsBlobSAS,
	})
	if err != nil {
		pool.Close()
		return nil, err
	}
	// For a local store, best-effort create the expected category folders so
	// operators know where to drop builds (no-op on a read-only mount or blob).
	if localSource, ok := artifactSource.(*artifacts.LocalSource); ok {
		localSource.EnsureLayout(append(append([]artifacts.Category{}, artifacts.LauncherCategories...), artifacts.ExtensionCategories...)...)
	}
	artifactService := artifacts.NewService(artifactSource, cfg.ChromeWebStoreURL, cfg.FirefoxExtensionURL)

	server := api.NewServer(api.Dependencies{
		Authenticator:    authService,
		Resources:        resourceService,
		KeyVault:         keyVaultService,
		AppRegistrations: appRegistrationService,
		Audit:            auditService,
		FrontendURL:      cfg.FrontendURL,
		AdminConfig:      adminConfigProvider{AdminConfigStore: adminStore},
		LocalGroups:      authService,
		Notifications:    notificationService,
		Artifacts:        artifactService,
	})

	application := &App{
		db:      pool,
		handler: server,
	}
	application.startAutomaticKeyVaultSync(adminStore, resourceService)
	return application, nil
}

func (a *App) Handler() *api.Server {
	return a.handler
}

func (a *App) Close() {
	if a.backgroundCancel != nil {
		a.backgroundCancel()
	}
	a.backgroundWG.Wait()
	if a.db != nil {
		a.db.Close()
	}
}
