package app

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"access-workspace/backend/internal/auth"
	"access-workspace/backend/internal/resources"
)

const automaticKeyVaultSyncInterval = time.Minute

var automaticKeyVaultSyncUser = auth.User{
	ID:      "system:keyvault-sync",
	Name:    "System sync",
	Email:   "system-sync@example.internal",
	IsAdmin: true,
}

type keyVaultSourceSyncStore interface {
	ListKeyVaultSources(ctx context.Context) ([]KeyVaultSource, error)
	UpdateKeyVaultSyncState(ctx context.Context, payload any) error
	GetAppRegistrationSyncSettings(ctx context.Context) (AppRegistrationSyncSettings, error)
	UpdateAppRegistrationSyncState(ctx context.Context, payload any) error
}

type keyVaultResourceSyncer interface {
	SyncKeyVault(ctx context.Context, user auth.User, sources []resources.KeyVaultSyncSourceConfig, automatic bool) (resources.KeyVaultSyncResult, error)
}

type appRegistrationResourceSyncer interface {
	SyncAppRegistrations(ctx context.Context, user auth.User, automatic bool) (resources.AppRegistrationSyncResult, error)
}

func (a *App) startAutomaticKeyVaultSync(store keyVaultSourceSyncStore, syncer keyVaultResourceSyncer) {
	ctx, cancel := context.WithCancel(context.Background())
	a.backgroundCancel = cancel
	a.backgroundWG.Add(1)

	go func() {
		defer a.backgroundWG.Done()

		runAutomaticKeyVaultSyncLoop(ctx, store, syncer)
	}()
}

func runAutomaticKeyVaultSyncLoop(ctx context.Context, store keyVaultSourceSyncStore, syncer keyVaultResourceSyncer) {
	ticker := time.NewTicker(automaticKeyVaultSyncInterval)
	defer ticker.Stop()

	runAutomaticKeyVaultSyncOnce(ctx, store, syncer)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runAutomaticKeyVaultSyncOnce(ctx, store, syncer)
		}
	}
}

func runAutomaticKeyVaultSyncOnce(ctx context.Context, store keyVaultSourceSyncStore, syncer keyVaultResourceSyncer) {
	if err := runAutomaticKeyVaultSync(ctx, store, syncer); err != nil && ctx.Err() == nil {
		log.Printf("automatic key vault sync: %v", err)
	}
	if appSyncer, ok := syncer.(appRegistrationResourceSyncer); ok {
		if err := runAutomaticAppRegistrationSync(ctx, store, appSyncer); err != nil && ctx.Err() == nil {
			log.Printf("automatic app registration sync: %v", err)
		}
	}
}

func runAutomaticKeyVaultSync(ctx context.Context, store keyVaultSourceSyncStore, syncer keyVaultResourceSyncer) error {
	sources, err := store.ListKeyVaultSources(ctx)
	if err != nil {
		return err
	}
	if len(sources) == 0 {
		return nil
	}

	result, err := syncer.SyncKeyVault(ctx, automaticKeyVaultSyncUser, toKeyVaultSyncSourceConfigs(sources), true)
	if err != nil {
		return err
	}

	return store.UpdateKeyVaultSyncState(ctx, keyVaultSyncStatePayload(result.Sources))
}

func toKeyVaultSyncSourceConfigs(sources []KeyVaultSource) []resources.KeyVaultSyncSourceConfig {
	items := make([]resources.KeyVaultSyncSourceConfig, 0, len(sources))
	for _, source := range sources {
		items = append(items, resources.KeyVaultSyncSourceConfig{
			Name:                 source.Name,
			VaultURL:             source.VaultURL,
			SyncEnabled:          source.SyncEnabled,
			SyncIntervalMinutes:  source.SyncIntervalMinutes,
			AutoImportEnabled:    source.AutoImportEnabled,
			DefaultOwner:         source.DefaultOwner,
			DefaultOwnerTeam:     source.DefaultOwnerTeam,
			DefaultEnvironment:   source.DefaultEnvironment,
			DefaultDescription:   source.DefaultDescription,
			DefaultNotes:         source.DefaultNotes,
			DefaultAllowedGroups: append([]string{}, source.DefaultAllowedGroups...),
			LastSyncedAt:         source.LastSyncedAt,
			LastSyncStatus:       source.LastSyncStatus,
			LastSyncError:        source.LastSyncError,
			LastSyncSummary:      source.LastSyncSummary,
		})
	}
	return items
}

func keyVaultSyncStatePayload(sources []resources.KeyVaultSyncSource) map[string]any {
	out := make(map[string]any, len(sources))
	for _, source := range sources {
		out[source.VaultURL] = map[string]any{
			"lastSyncedAt":    source.LastSyncedAt,
			"lastSyncStatus":  source.LastSyncStatus,
			"lastSyncError":   source.LastSyncError,
			"lastSyncSummary": source.LastSyncSummary,
		}
	}
	return out
}

func runAutomaticAppRegistrationSync(ctx context.Context, store keyVaultSourceSyncStore, syncer appRegistrationResourceSyncer) error {
	settings, err := store.GetAppRegistrationSyncSettings(ctx)
	if err != nil {
		return err
	}
	if !settings.Enabled {
		return nil
	}
	now := time.Now().UTC()
	if settings.State.LastSyncedAt != nil && settings.IntervalMinutes > 0 &&
		settings.State.LastSyncedAt.Add(time.Duration(settings.IntervalMinutes)*time.Minute).After(now) {
		return nil
	}

	result, err := syncer.SyncAppRegistrations(ctx, automaticKeyVaultSyncUser, true)
	state := AppRegistrationSyncState{
		LastSyncedAt:    &now,
		LastSyncStatus:  "ok",
		LastSyncSummary: summarizeAutomaticAppRegistrationSync(result),
	}
	if err != nil {
		state.LastSyncStatus = "error"
		state.LastSyncError = err.Error()
		state.LastSyncSummary = ""
		if updateErr := store.UpdateAppRegistrationSyncState(ctx, state); updateErr != nil {
			log.Printf("automatic app registration sync state update failed: %v", updateErr)
		}
		return err
	}
	return store.UpdateAppRegistrationSyncState(ctx, state)
}

func summarizeAutomaticAppRegistrationSync(result resources.AppRegistrationSyncResult) string {
	if result.AttemptedResources == 0 {
		return "No imported app registrations needed syncing."
	}
	parts := []string{fmt.Sprintf("Updated %d app registrations", result.UpdatedResources)}
	if result.RemovedResources > 0 {
		parts = append(parts, fmt.Sprintf("Removed %d missing apps", result.RemovedResources))
	}
	if result.ExpiringCredentials > 0 {
		parts = append(parts, fmt.Sprintf("%d credentials expiring soon", result.ExpiringCredentials))
	}
	if result.ExpiredCredentials > 0 {
		parts = append(parts, fmt.Sprintf("%d credentials expired", result.ExpiredCredentials))
	}
	if result.MissingResources > 0 {
		parts = append(parts, fmt.Sprintf("%d need attention", result.MissingResources))
	}
	return strings.Join(parts, ", ")
}
