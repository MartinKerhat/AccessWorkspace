package resources

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MartinKerhat/AccessWorkspace/backend/internal/audit"
	"github.com/MartinKerhat/AccessWorkspace/backend/internal/auth"
	"github.com/MartinKerhat/AccessWorkspace/backend/internal/keyvault"
)

const archivedTestVault = "https://vault.example.vault.azure.net"

func discoveredSecret(name string) fakeKeyVaultResolver {
	return fakeKeyVaultResolver{
		discover: keyvault.DiscoverResult{Sources: []keyvault.DiscoverSourceResult{{
			Source: keyvault.Source{VaultURL: archivedTestVault, Name: "example"},
			Items: []keyvault.SecretItem{{
				ID:        archivedTestVault + "/secrets/" + name,
				Name:      name,
				VaultName: "example",
				VaultURL:  archivedTestVault,
				Version:   "v1",
				Enabled:   true,
			}},
		}}},
		metadata: keyvault.SecretItem{ID: archivedTestVault + "/secrets/" + name, Name: name, VaultName: "example", Version: "v1", Enabled: true},
	}
}

func autoImportSource() []KeyVaultSyncSourceConfig {
	return []KeyVaultSyncSourceConfig{{VaultURL: archivedTestVault, AutoImportEnabled: true, DefaultOwner: "Alice Admin"}}
}

func archivedRow(reason string) ArchivedKeyVaultRow {
	return ArchivedKeyVaultRow{ID: "archived-1", Name: "api-key", Reference: archivedTestVault + "/secrets/api-key/", Reason: reason}
}

var syncAdmin = auth.User{ID: "admin", Name: "Admin", IsAdmin: true}

func TestAutoImportSkipsSecretArchivedByAUser(t *testing.T) {
	for _, reason := range []string{archiveReasonRemovedFromApp, "duplicate_key_vault_object", ""} {
		store := &fakeResourceStore{archivedKeyVault: []ArchivedKeyVaultRow{archivedRow(reason)}}
		service := NewService(store, fakeAuditLogger{}, discoveredSecret("api-key"), nil, nil)

		result, err := service.SyncKeyVault(context.Background(), syncAdmin, autoImportSource(), false)
		if err != nil {
			t.Fatalf("%q: sync: %v", reason, err)
		}
		if len(store.created) != 0 || len(store.restored) != 0 {
			t.Fatalf("%q: an archived object must stay archived and not be imported again, created=%d restored=%v", reason, len(store.created), store.restored)
		}
		if result.SkippedResources != 1 || result.Sources[0].SkippedCount != 1 {
			t.Fatalf("%q: expected the skip to be counted, got %#v", reason, result)
		}
	}
}

func TestAutoImportRestoresObjectWhoseSecretReappeared(t *testing.T) {
	restored := keyVaultRow()
	restored.ID = "archived-1"
	restored.Secret.Reference = archivedTestVault + "/secrets/api-key"
	store := &fakeResourceStore{
		archivedKeyVault: []ArchivedKeyVaultRow{archivedRow(archiveReasonKeyVaultNotFound)},
		byID:             map[string]Resource{"archived-1": restored},
	}
	auditLog := &recordingAudit{}
	service := NewService(store, auditLog, discoveredSecret("api-key"), nil, nil)

	// An automatic run only touches sources with sync on that are due.
	sources := autoImportSource()
	sources[0].SyncEnabled = true
	result, err := service.SyncKeyVault(context.Background(), syncAdmin, sources, true)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(store.created) != 0 {
		t.Fatalf("a reappeared secret must bring back its object, not create a second one")
	}
	if len(store.restored) != 1 || store.restored[0] != "archived-1" || result.RestoredResources != 1 {
		t.Fatalf("expected archived-1 restored, got %v %#v", store.restored, result)
	}
	if len(store.updated) != 1 {
		t.Fatalf("the restored object must be refreshed from Azure in the same run, got %d updates", len(store.updated))
	}
	if len(auditLog.events) == 0 || auditLog.events[0].EventType != audit.EventResourceRestored || auditLog.events[0].Metadata["reason"] != "key_vault_secret_reappeared" {
		t.Fatalf("expected a restored event with the reason, got %#v", auditLog.events)
	}
}

func TestManualImportSkipsUserArchivedAndRestoresReappeared(t *testing.T) {
	user := auth.User{ID: "admin", Name: "Admin", IsAdmin: true}
	reappeared := ArchivedKeyVaultRow{ID: "gone-1", Name: "db-password", Reference: archivedTestVault + "/secrets/db-password", Reason: archiveReasonKeyVaultNotFound}
	store := &fakeResourceStore{
		archivedKeyVault: []ArchivedKeyVaultRow{archivedRow(archiveReasonRemovedFromApp), reappeared},
		byID:             map[string]Resource{"gone-1": {ID: "gone-1", Name: "db-password"}},
	}
	service := NewService(store, fakeAuditLogger{}, fakeKeyVaultResolver{}, nil, nil)
	now := time.Now().UTC()

	result, err := service.ImportKeyVaultSecrets(context.Background(), user, KeyVaultImportInput{Items: []KeyVaultImportItem{
		{VaultURL: archivedTestVault, VaultName: "example", ObjectName: "api-key", SecretID: archivedTestVault + "/secrets/api-key", ExpiresAt: &now},
		{VaultURL: archivedTestVault, VaultName: "example", ObjectName: "db-password", SecretID: archivedTestVault + "/secrets/db-password"},
	}})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(store.created) != 0 || result.Archived != 1 || result.Restored != 1 {
		t.Fatalf("expected one skipped as archived and one restored, nothing created; got created=%d %#v", len(store.created), result)
	}
	if len(store.restored) != 1 || store.restored[0] != "gone-1" {
		t.Fatalf("expected gone-1 restored, got %v", store.restored)
	}
}

func TestRestoreRefusesWhenAnotherActiveObjectTracksTheSecret(t *testing.T) {
	archived := keyVaultRow()
	archived.ID = "old"
	archivedAt := time.Now().UTC()
	archived.ArchivedAt = &archivedAt
	active := keyVaultRow()
	active.ID = "new"
	active.Name = "api-key (re-imported)"
	store := &fakeResourceStore{managed: []Resource{active}, byID: map[string]Resource{"old": archived}}
	service := NewService(store, fakeAuditLogger{}, fakeKeyVaultResolver{}, nil, nil)

	err := service.Restore(context.Background(), auth.User{ID: "admin", IsAdmin: true}, "old")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected the restore to be refused, got %v", err)
	}
	if len(store.restored) != 0 {
		t.Fatalf("nothing may be restored next to an active twin")
	}

	store.managed = nil
	if err := service.Restore(context.Background(), auth.User{ID: "admin", IsAdmin: true}, "old"); err != nil || len(store.restored) != 1 {
		t.Fatalf("without a twin the restore goes through, got %v %v", err, store.restored)
	}
}
