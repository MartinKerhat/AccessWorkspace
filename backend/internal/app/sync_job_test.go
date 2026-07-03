package app

import (
	"context"
	"testing"
	"time"

	"access-workspace/backend/internal/auth"
	"access-workspace/backend/internal/resources"
)

type fakeKeyVaultSyncStore struct {
	sources  []KeyVaultSource
	payload  any
	updateOK bool
}

func (s *fakeKeyVaultSyncStore) ListKeyVaultSources(context.Context) ([]KeyVaultSource, error) {
	return s.sources, nil
}

func (s *fakeKeyVaultSyncStore) UpdateKeyVaultSyncState(_ context.Context, payload any) error {
	s.payload = payload
	s.updateOK = true
	return nil
}

func (s *fakeKeyVaultSyncStore) GetAppRegistrationSyncSettings(context.Context) (AppRegistrationSyncSettings, error) {
	return AppRegistrationSyncSettings{Enabled: false, IntervalMinutes: 60}, nil
}

func (s *fakeKeyVaultSyncStore) UpdateAppRegistrationSyncState(context.Context, any) error {
	return nil
}

type fakeKeyVaultSyncer struct {
	called    bool
	user      auth.User
	sources   []resources.KeyVaultSyncSourceConfig
	automatic bool
	result    resources.KeyVaultSyncResult
}

func (s *fakeKeyVaultSyncer) SyncKeyVault(_ context.Context, user auth.User, sources []resources.KeyVaultSyncSourceConfig, automatic bool) (resources.KeyVaultSyncResult, error) {
	s.called = true
	s.user = user
	s.sources = sources
	s.automatic = automatic
	return s.result, nil
}

func TestRunAutomaticKeyVaultSyncUsesAutomaticModeAndPersistsState(t *testing.T) {
	now := time.Date(2026, time.May, 25, 8, 30, 0, 0, time.UTC)
	store := &fakeKeyVaultSyncStore{
		sources: []KeyVaultSource{{
			Name:                "kvdemo",
			VaultURL:            "https://kvdemo.vault.azure.net",
			SyncEnabled:         true,
			SyncIntervalMinutes: 5,
			AutoImportEnabled:   true,
			DefaultOwner:        "Alice Admin",
		}},
	}
	syncer := &fakeKeyVaultSyncer{
		result: resources.KeyVaultSyncResult{
			Automatic: true,
			Sources: []resources.KeyVaultSyncSource{{
				Name:            "kvdemo",
				VaultURL:        "https://kvdemo.vault.azure.net",
				SyncEnabled:     true,
				LastSyncedAt:    &now,
				LastSyncStatus:  "ok",
				LastSyncSummary: "Updated 1 imported resources",
			}},
		},
	}

	if err := runAutomaticKeyVaultSync(context.Background(), store, syncer); err != nil {
		t.Fatalf("expected automatic sync to succeed, got %v", err)
	}
	if !syncer.called {
		t.Fatal("expected syncer to be called")
	}
	if !syncer.automatic {
		t.Fatal("expected automatic sync mode to be true")
	}
	if syncer.user.ID != automaticKeyVaultSyncUser.ID || !syncer.user.IsAdmin {
		t.Fatalf("expected automatic sync user to be used, got %#v", syncer.user)
	}
	if len(syncer.sources) != 1 || syncer.sources[0].DefaultOwner != "Alice Admin" {
		t.Fatalf("expected source config to be forwarded, got %#v", syncer.sources)
	}
	if !store.updateOK {
		t.Fatal("expected sync state to be persisted")
	}
	payload, ok := store.payload.(map[string]any)
	if !ok {
		t.Fatalf("expected sync state payload to be a map, got %T", store.payload)
	}
	entry, ok := payload["https://kvdemo.vault.azure.net"].(map[string]any)
	if !ok {
		t.Fatalf("expected keyed sync state entry, got %#v", payload)
	}
	if entry["lastSyncStatus"] != "ok" {
		t.Fatalf("expected sync status to be written, got %#v", entry)
	}
}
