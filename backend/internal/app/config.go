package app

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// legacyDevSecretKey is the old hardcoded encryption key that used to ship as a
// default. It is rejected explicitly so a deployment can never silently run with
// a key that is public knowledge.
const legacyDevSecretKey = "access-workspace-local-dev-key"

// envProduction is the only APP_ENV value that switches on runtime protection
// (blocking destructive/dev-only flags). Any other value — CI, staging,
// preprod, test, empty — runs without those restrictions.
const envProduction = "production"

type Config struct {
	AppEnv              string
	HTTPAddr            string
	DatabaseURL         string
	FrontendURL         string
	AutoMigrate         bool
	ResetDBOnStart      bool
	SeedOnStart         bool
	DefaultUser         string
	AuthMode            string
	SecretEncryptionKey string
	KEKProvider         string
	KEKVaultURL         string
	KEKKeyName          string
	BootstrapAdminUser  string
	BootstrapAdminPass  string
	BootstrapAdminName  string
	BootstrapAdminEmail string
	ArtifactsSource     string
	ArtifactsDir        string
	ArtifactsBlobURL    string
	ArtifactsBlobSAS    string
	ArtifactsGitHubRepo string
	ArtifactsGitHubTok  string
	ChromeWebStoreURL   string
	FirefoxExtensionURL string
	EntraTenantID       string
	EntraClientID       string
	EntraAuthority      string
	EntraRedirectURI    string
	EntraGroupSource    string
	EntraClientSecret   string
	EntraDirectRights   string
	// Session lifetimes as Go durations ("24h", "720h"); empty keeps the
	// built-in defaults (web 24h, browser extension 30 days). Parsed and
	// validated in Validate; read through SessionTTL/BrowserExtensionSessionTTL.
	SessionTTLRaw                 string
	BrowserExtensionSessionTTLRaw string
}

func ConfigFromEnv() Config {
	return Config{
		AppEnv:              strings.TrimSpace(os.Getenv("APP_ENV")),
		HTTPAddr:            envOrDefault("HTTP_ADDR", ":8080"),
		DatabaseURL:         envOrDefault("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/access_workspace?sslmode=disable"),
		FrontendURL:         envOrDefault("FRONTEND_URL", "http://localhost:5173"),
		AutoMigrate:         envOrDefault("AUTO_MIGRATE", "true") == "true",
		ResetDBOnStart:      envOrDefault("RESET_DB_ON_START", "false") == "true",
		SeedOnStart:         envOrDefault("SEED_ON_START", "false") == "true",
		DefaultUser:         envOrDefault("DEFAULT_DEV_USER", "alice"),
		AuthMode:            envOrDefault("AUTH_MODE", "dev"),
		SecretEncryptionKey: strings.TrimSpace(os.Getenv("RESOURCE_SECRET_KEY")),
		KEKProvider:         strings.ToLower(envOrDefault("KEK_PROVIDER", "local")),
		KEKVaultURL:         strings.TrimSpace(os.Getenv("KEK_VAULT_URL")),
		KEKKeyName:          envOrDefault("KEK_KEY_NAME", "access-workspace-kek"),
		BootstrapAdminUser:  strings.TrimSpace(os.Getenv("BOOTSTRAP_ADMIN_USERNAME")),
		BootstrapAdminPass:  strings.TrimSpace(os.Getenv("BOOTSTRAP_ADMIN_PASSWORD")),
		BootstrapAdminName:  strings.TrimSpace(os.Getenv("BOOTSTRAP_ADMIN_DISPLAY_NAME")),
		BootstrapAdminEmail: strings.TrimSpace(os.Getenv("BOOTSTRAP_ADMIN_EMAIL")),
		ArtifactsSource:     strings.ToLower(envOrDefault("ARTIFACTS_SOURCE", "local")),
		ArtifactsDir:        envOrDefault("ARTIFACTS_DIR", "/data/downloads"),
		ArtifactsBlobURL:    strings.TrimSpace(os.Getenv("ARTIFACTS_BLOB_CONTAINER_URL")),
		ArtifactsBlobSAS:    strings.TrimSpace(os.Getenv("ARTIFACTS_BLOB_SAS")),
		ArtifactsGitHubRepo: strings.TrimSpace(os.Getenv("ARTIFACTS_GITHUB_REPO")),
		ArtifactsGitHubTok:  strings.TrimSpace(os.Getenv("ARTIFACTS_GITHUB_TOKEN")),
		ChromeWebStoreURL:   strings.TrimSpace(os.Getenv("CHROME_WEB_STORE_URL")),
		FirefoxExtensionURL: strings.TrimSpace(os.Getenv("FIREFOX_EXTENSION_URL")),
		EntraTenantID:       envOrDefault("ENTRA_TENANT_ID", ""),
		EntraClientID:       envOrDefault("ENTRA_CLIENT_ID", ""),
		EntraAuthority:      envOrDefault("ENTRA_AUTHORITY", "https://login.microsoftonline.com"),
		EntraRedirectURI:    envOrDefault("ENTRA_REDIRECT_URI", "http://localhost:8080/api/auth/microsoft/callback"),
		EntraGroupSource:    envOrDefault("ENTRA_GROUP_SOURCE", "graph"),
		EntraClientSecret:   strings.TrimSpace(os.Getenv("ENTRA_CLIENT_SECRET")),
		EntraDirectRights:   envOrDefault("ENTRA_DIRECT_RIGHTS_JSON", ""),

		SessionTTLRaw:                 strings.TrimSpace(os.Getenv("SESSION_TTL")),
		BrowserExtensionSessionTTLRaw: strings.TrimSpace(os.Getenv("BROWSER_EXTENSION_SESSION_TTL")),
	}
}

// parseSessionTTL accepts an empty value (use the default) or a positive Go
// duration of at least one minute. Anything else is a configuration error.
func parseSessionTTL(name, raw string) (time.Duration, error) {
	if raw == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a Go duration such as \"24h\" or \"720h\", got %q", name, raw)
	}
	if d < time.Minute {
		return 0, fmt.Errorf("%s must be at least 1m, got %q", name, raw)
	}
	return d, nil
}

// SessionTTL is the configured web-session lifetime, or 0 for the default.
func (c Config) SessionTTL() time.Duration {
	d, _ := parseSessionTTL("SESSION_TTL", c.SessionTTLRaw)
	return d
}

// BrowserExtensionSessionTTL is the configured extension-session lifetime, or
// 0 for the default.
func (c Config) BrowserExtensionSessionTTL() time.Duration {
	d, _ := parseSessionTTL("BROWSER_EXTENSION_SESSION_TTL", c.BrowserExtensionSessionTTLRaw)
	return d
}

// Validate ensures every secret the app depends on is provided by the
// environment rather than a baked-in default. It fails fast at startup so a
// misconfigured deployment never runs with a publicly known key.
func (c Config) Validate() error {
	if c.SecretEncryptionKey == "" {
		return fmt.Errorf("RESOURCE_SECRET_KEY is required and must be provided via the environment (set it in .env for local dev, or via a Kubernetes Secret in production)")
	}
	if c.SecretEncryptionKey == legacyDevSecretKey {
		return fmt.Errorf("RESOURCE_SECRET_KEY must not be the legacy shared development key; generate a unique secret per deployment")
	}

	switch c.KEKProvider {
	case "", "local":
		// Key derived from RESOURCE_SECRET_KEY; no further config.
	case "azure_key_vault":
		if c.KEKVaultURL == "" {
			return fmt.Errorf("KEK_VAULT_URL is required when KEK_PROVIDER=azure_key_vault")
		}
		if c.KEKKeyName == "" {
			return fmt.Errorf("KEK_KEY_NAME is required when KEK_PROVIDER=azure_key_vault")
		}
	default:
		return fmt.Errorf("KEK_PROVIDER must be \"local\" or \"azure_key_vault\", got %q", c.KEKProvider)
	}

	// A bootstrap admin needs both a username and a password, or neither.
	if (c.BootstrapAdminUser == "") != (c.BootstrapAdminPass == "") {
		return fmt.Errorf("BOOTSTRAP_ADMIN_USERNAME and BOOTSTRAP_ADMIN_PASSWORD must be set together")
	}
	if c.BootstrapAdminPass != "" && len(c.BootstrapAdminPass) < 8 {
		return fmt.Errorf("BOOTSTRAP_ADMIN_PASSWORD must be at least 8 characters")
	}

	switch c.ArtifactsSource {
	case "local":
		if c.ArtifactsDir == "" {
			return fmt.Errorf("ARTIFACTS_DIR is required when ARTIFACTS_SOURCE=local")
		}
	case "blob":
		if c.ArtifactsBlobURL == "" {
			return fmt.Errorf("ARTIFACTS_BLOB_CONTAINER_URL is required when ARTIFACTS_SOURCE=blob")
		}
	case "github":
		if c.ArtifactsGitHubRepo == "" || !strings.Contains(c.ArtifactsGitHubRepo, "/") {
			return fmt.Errorf("ARTIFACTS_GITHUB_REPO (owner/name) is required when ARTIFACTS_SOURCE=github")
		}
	default:
		return fmt.Errorf("ARTIFACTS_SOURCE must be \"local\", \"blob\", or \"github\", got %q", c.ArtifactsSource)
	}

	if c.IsProduction() {
		if c.SeedOnStart {
			return fmt.Errorf("SEED_ON_START must be false when APP_ENV=production (demo data must not be seeded into production)")
		}
		if c.ResetDBOnStart {
			return fmt.Errorf("RESET_DB_ON_START must be false when APP_ENV=production (it would wipe the database)")
		}
	}

	if _, err := parseSessionTTL("SESSION_TTL", c.SessionTTLRaw); err != nil {
		return err
	}
	if _, err := parseSessionTTL("BROWSER_EXTENSION_SESSION_TTL", c.BrowserExtensionSessionTTLRaw); err != nil {
		return err
	}

	if c.AuthMode == "entra" {
		var missing []string
		if c.EntraTenantID == "" {
			missing = append(missing, "ENTRA_TENANT_ID")
		}
		if c.EntraClientID == "" {
			missing = append(missing, "ENTRA_CLIENT_ID")
		}
		if c.EntraClientSecret == "" {
			missing = append(missing, "ENTRA_CLIENT_SECRET")
		}
		if len(missing) > 0 {
			return fmt.Errorf("AUTH_MODE=entra requires: %s", strings.Join(missing, ", "))
		}
	}

	return nil
}

// IsProduction reports whether the app is running in production. Only the exact
// value "production" (case-insensitive) enables the protections; every other
// value — including empty — is treated as non-production.
func (c Config) IsProduction() bool {
	return strings.EqualFold(strings.TrimSpace(c.AppEnv), envProduction)
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
