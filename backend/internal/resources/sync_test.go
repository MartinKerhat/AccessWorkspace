package resources

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"access-workspace/backend/internal/appregistrations"
	"access-workspace/backend/internal/audit"
	"access-workspace/backend/internal/auth"
	"access-workspace/backend/internal/keyvault"
)

type fakeResourceStore struct {
	managed         []Resource
	managedAppRegs  []Resource
	archive         []string
	created         []CreateResourceInput
	updated         []UpdateResourceInput
	appCredentials  []AppRegistrationCredential
	appOwners       []AppRegistrationOwner
	snapshotUpdates int
}

func (s *fakeResourceStore) List(context.Context, Filter) ([]ResourceSummary, error) {
	return nil, nil
}

func (s *fakeResourceStore) Get(context.Context, string) (Resource, error) {
	return Resource{}, ErrNotFound
}

func (s *fakeResourceStore) GetAny(context.Context, string) (Resource, error) {
	return Resource{}, ErrNotFound
}

func (s *fakeResourceStore) ListArchived(context.Context) ([]ArchivedResourceSummary, error) {
	return nil, nil
}

func (s *fakeResourceStore) ListManagedKeyVault(context.Context) ([]Resource, error) {
	return s.managed, nil
}

func (s *fakeResourceStore) ListManagedAppRegistrations(context.Context) ([]Resource, error) {
	return s.managedAppRegs, nil
}

func (s *fakeResourceStore) Create(_ context.Context, input CreateResourceInput) (Resource, error) {
	s.created = append(s.created, input)
	return Resource{
		ID:                  "created-resource",
		Name:                input.Name,
		Type:                input.Type,
		SourceKind:          input.SourceKind,
		SourceObjectID:      input.SourceObjectID,
		ApplicationID:       input.ApplicationID,
		CredentialType:      input.CredentialType,
		CredentialExpiresAt: input.CredentialExpiresAt,
		Secret: Secret{
			Mode:      input.SecretMode,
			Reference: input.SecretReference,
		},
	}, nil
}

func (s *fakeResourceStore) Update(_ context.Context, _ string, input UpdateResourceInput) (Resource, error) {
	s.updated = append(s.updated, input)
	return Resource{}, nil
}

func (s *fakeResourceStore) Archive(_ context.Context, id string) error {
	s.archive = append(s.archive, id)
	return nil
}

func (s *fakeResourceStore) Delete(context.Context, string) error { return nil }

func (s *fakeResourceStore) Restore(context.Context, string) error {
	return nil
}

func (s *fakeResourceStore) GetConnectionUserPasswordOverride(context.Context, string, string) (ConnectionCredentialOverride, error) {
	return ConnectionCredentialOverride{}, ErrNotFound
}

func (s *fakeResourceStore) UpsertConnectionUserPasswordOverride(context.Context, string, string, string) error {
	return nil
}

func (s *fakeResourceStore) DeleteConnectionUserPasswordOverride(context.Context, string, string) error {
	return nil
}

func (s *fakeResourceStore) ReplaceAppRegistrationSnapshot(_ context.Context, _ string, credentials []AppRegistrationCredential, owners []AppRegistrationOwner) error {
	s.snapshotUpdates++
	s.appCredentials = append([]AppRegistrationCredential{}, credentials...)
	s.appOwners = append([]AppRegistrationOwner{}, owners...)
	return nil
}

func (s *fakeResourceStore) ReplaceAppRegistrationNotificationPolicies(context.Context, string, *AppRegistrationNotificationPolicy, []AppRegistrationCredentialPolicyInput) error {
	return nil
}

type fakeAuditLogger struct{}

func (fakeAuditLogger) Log(context.Context, audit.LogParams) error {
	return nil
}

type fakeKeyVaultResolver struct {
	err      error
	discover keyvault.DiscoverResult
	metadata keyvault.SecretItem
}

func (r fakeKeyVaultResolver) RevealSecret(context.Context, string) (string, error) {
	return "", nil
}

func (r fakeKeyVaultResolver) Discover(context.Context) (keyvault.DiscoverResult, error) {
	return r.discover, nil
}

func (r fakeKeyVaultResolver) CurrentSecretMetadata(context.Context, string) (keyvault.SecretItem, error) {
	if r.err != nil {
		return keyvault.SecretItem{}, r.err
	}
	return r.metadata, nil
}

type fakeAppRegistrationResolver struct {
	err         error
	discover    appregistrations.DiscoverResult
	application appregistrations.ApplicationItem
}

func (r fakeAppRegistrationResolver) Discover(context.Context) (appregistrations.DiscoverResult, error) {
	return r.discover, nil
}

func (r fakeAppRegistrationResolver) CurrentApplication(context.Context, string) (appregistrations.ApplicationItem, error) {
	if r.err != nil {
		return appregistrations.ApplicationItem{}, r.err
	}
	return r.application, nil
}

func TestSyncKeyVaultAutoImportsMissingSecrets(t *testing.T) {
	store := &fakeResourceStore{}
	resolver := fakeKeyVaultResolver{
		discover: keyvault.DiscoverResult{
			Sources: []keyvault.DiscoverSourceResult{{
				Source: keyvault.Source{VaultURL: "https://vault.example.vault.azure.net", Name: "example"},
				Items: []keyvault.SecretItem{{
					ID:          "https://vault.example.vault.azure.net/secrets/new-secret",
					Name:        "new-secret",
					VaultName:   "example",
					VaultURL:    "https://vault.example.vault.azure.net",
					ContentType: "text/plain",
					Version:     "v1",
				}},
			}},
		},
	}
	service := NewService(store, fakeAuditLogger{}, resolver, nil, nil)

	result, err := service.SyncKeyVault(context.Background(), auth.User{ID: "admin", Name: "Admin", IsAdmin: true}, []KeyVaultSyncSourceConfig{{
		VaultURL:             "https://vault.example.vault.azure.net",
		AutoImportEnabled:    true,
		DefaultOwner:         "Alice Admin",
		DefaultOwnerTeam:     "support",
		DefaultEnvironment:   "prod",
		DefaultDescription:   "Auto imported from Key Vault",
		DefaultNotes:         "Managed by auto import",
		DefaultAllowedGroups: []string{"engineering"},
	}}, false)
	if err != nil {
		t.Fatalf("expected sync to succeed, got %v", err)
	}
	if result.ImportedResources != 1 {
		t.Fatalf("expected one imported resource, got %#v", result)
	}
	if len(store.created) != 1 {
		t.Fatalf("expected one create call, got %#v", store.created)
	}
	if store.created[0].Owner != "Alice Admin" || store.created[0].OwnerTeam != "support" {
		t.Fatalf("expected auto import defaults to be applied, got %#v", store.created[0])
	}
	if len(store.created[0].AllowedGroups) != 1 || store.created[0].AllowedGroups[0] != "engineering" {
		t.Fatalf("expected auto import groups to be applied, got %#v", store.created[0].AllowedGroups)
	}
}

func TestSyncKeyVaultArchivesSecretsDeletedFromAzure(t *testing.T) {
	store := &fakeResourceStore{
		managed: []Resource{
			{
				ID:         "res-1",
				Name:       "Deleted secret",
				Type:       TypeKeyVaultSecret,
				SourceKind: SourceKindAzureKeyVault,
				Secret: Secret{
					Reference: "https://vault.example.vault.azure.net/secrets/deleted",
				},
			},
		},
	}
	service := NewService(store, fakeAuditLogger{}, fakeKeyVaultResolver{
		err: keyvault.RequestError{StatusCode: http.StatusNotFound, Body: "not found"},
	}, nil, nil)

	result, err := service.SyncKeyVault(context.Background(), auth.User{ID: "admin", Name: "Admin", IsAdmin: true}, []KeyVaultSyncSourceConfig{
		{VaultURL: "https://vault.example.vault.azure.net"},
	}, false)
	if err != nil {
		t.Fatalf("expected sync to succeed, got %v", err)
	}
	if result.RemovedResources != 1 {
		t.Fatalf("expected one removed resource, got %#v", result)
	}
	if len(store.archive) != 1 || store.archive[0] != "res-1" {
		t.Fatalf("expected deleted secret to be archived, got %#v", store.archive)
	}
}

func TestSyncKeyVaultDoesNotArchiveOnTransientErrors(t *testing.T) {
	store := &fakeResourceStore{
		managed: []Resource{
			{
				ID:         "res-1",
				Name:       "Transient secret",
				Type:       TypeKeyVaultSecret,
				SourceKind: SourceKindAzureKeyVault,
				Secret: Secret{
					Reference: "https://vault.example.vault.azure.net/secrets/transient",
				},
			},
		},
	}
	service := NewService(store, fakeAuditLogger{}, fakeKeyVaultResolver{
		err: fmt.Errorf("temporary network failure"),
	}, nil, nil)

	result, err := service.SyncKeyVault(context.Background(), auth.User{ID: "admin", Name: "Admin", IsAdmin: true}, []KeyVaultSyncSourceConfig{
		{VaultURL: "https://vault.example.vault.azure.net"},
	}, false)
	if err != nil {
		t.Fatalf("expected sync to finish with per-source error state, got %v", err)
	}
	if result.RemovedResources != 0 {
		t.Fatalf("expected no removed resources, got %#v", result)
	}
	if len(store.archive) != 0 {
		t.Fatalf("expected transient error not to archive resources, got %#v", store.archive)
	}
}

func TestSyncKeyVaultMarksDisabledSecretsWithoutArchiving(t *testing.T) {
	store := &fakeResourceStore{
		managed: []Resource{
			{
				ID:         "res-1",
				Name:       "Disabled secret",
				Type:       TypeKeyVaultSecret,
				SourceKind: SourceKindAzureKeyVault,
				Secret: Secret{
					Mode:      SecretModeExternal,
					Reference: "https://vault.example.vault.azure.net/secrets/disabled",
				},
			},
		},
	}
	service := NewService(store, fakeAuditLogger{}, fakeKeyVaultResolver{
		metadata: keyvault.SecretItem{
			ID:        "https://vault.example.vault.azure.net/secrets/disabled",
			Name:      "disabled",
			VaultName: "example",
			VaultURL:  "https://vault.example.vault.azure.net",
			Enabled:   false,
			Version:   "v1",
		},
	}, nil, nil)

	result, err := service.SyncKeyVault(context.Background(), auth.User{ID: "admin", Name: "Admin", IsAdmin: true}, []KeyVaultSyncSourceConfig{
		{VaultURL: "https://vault.example.vault.azure.net"},
	}, false)
	if err != nil {
		t.Fatalf("expected sync to succeed, got %v", err)
	}
	if result.UpdatedResources != 1 {
		t.Fatalf("expected one updated resource, got %#v", result)
	}
	if len(store.archive) != 0 {
		t.Fatalf("expected disabled secret not to be archived, got %#v", store.archive)
	}
	if len(store.updated) != 1 || store.updated[0].Status != "disabled" {
		t.Fatalf("expected disabled status to be synced, got %#v", store.updated)
	}
}

func TestSyncKeyVaultRefreshesPerSourceSyncStateOnLaterSuccess(t *testing.T) {
	previous := time.Date(2026, time.May, 22, 6, 56, 55, 0, time.UTC)
	store := &fakeResourceStore{
		managed: []Resource{
			{
				ID:         "res-1",
				Name:       "Existing secret",
				Type:       TypeKeyVaultSecret,
				SourceKind: SourceKindAzureKeyVault,
				VaultName:  "example",
				ObjectName: "existing-secret",
				ObjectType: "secret",
				Secret: Secret{
					Mode:      SecretModeExternal,
					Reference: "https://vault.example.vault.azure.net/secrets/existing-secret",
				},
			},
		},
	}
	service := NewService(store, fakeAuditLogger{}, fakeKeyVaultResolver{
		metadata: keyvault.SecretItem{
			ID:          "https://vault.example.vault.azure.net/secrets/existing-secret",
			Name:        "existing-secret",
			VaultName:   "example",
			VaultURL:    "https://vault.example.vault.azure.net",
			ContentType: "text/plain",
			Version:     "v2",
		},
	}, nil, nil)

	result, err := service.SyncKeyVault(context.Background(), auth.User{ID: "admin", Name: "Admin", IsAdmin: true}, []KeyVaultSyncSourceConfig{{
		VaultURL:        "https://vault.example.vault.azure.net",
		LastSyncedAt:    &previous,
		LastSyncStatus:  "ok",
		LastSyncSummary: "Updated 5 imported resources",
	}}, false)
	if err != nil {
		t.Fatalf("expected sync to succeed, got %v", err)
	}
	if len(store.updated) != 1 {
		t.Fatalf("expected managed secret metadata to be updated, got %#v", store.updated)
	}
	if result.Sources[0].LastSyncedAt == nil || !result.Sources[0].LastSyncedAt.After(previous) {
		t.Fatalf("expected last synced to advance, got %#v", result.Sources[0].LastSyncedAt)
	}
	if result.Sources[0].LastSyncStatus != "ok" {
		t.Fatalf("expected ok sync status, got %#v", result.Sources[0].LastSyncStatus)
	}
	if !strings.Contains(result.Sources[0].LastSyncSummary, "Updated 1 imported resources") {
		t.Fatalf("expected fresh sync summary, got %#v", result.Sources[0].LastSyncSummary)
	}
}

func TestImportAppRegistrationsStoresCredentialSnapshotAndNextExpiry(t *testing.T) {
	secretExpiry := time.Now().UTC().Add(20 * 24 * time.Hour)
	certExpiry := time.Now().UTC().Add(120 * 24 * time.Hour)
	store := &fakeResourceStore{}
	service := NewService(store, fakeAuditLogger{}, fakeKeyVaultResolver{}, fakeAppRegistrationResolver{
		application: appregistrations.ApplicationItem{
			ID:          "app-object-1",
			AppID:       "client-id-1",
			DisplayName: "Billing Worker",
			Credentials: []appregistrations.CredentialItem{
				{
					KeyID:       "secret-key-1",
					DisplayName: "prod secret",
					Type:        "client_secret",
					EndDateTime: &secretExpiry,
					Hint:        "abc",
				},
				{
					KeyID:       "cert-key-1",
					DisplayName: "prod cert",
					Type:        "certificate",
					EndDateTime: &certExpiry,
					Usage:       "Verify",
				},
			},
			Owners: []appregistrations.OwnerItem{
				{ID: "owner-1", Type: "user", DisplayName: "Alice Admin", Email: "alice@example.internal"},
			},
		},
	}, nil)

	items, err := service.ImportAppRegistrations(context.Background(), auth.User{ID: "admin", Name: "Admin", IsAdmin: true}, AppRegistrationImportInput{
		Owner:          "Alice Admin",
		OwnerTeam:      "platform",
		Environment:    "prod",
		TenantID:       "tenant-1",
		ApplicationIDs: []string{"app-object-1"},
		AllowedGroups:  []string{"platform"},
	})
	if err != nil {
		t.Fatalf("expected import to succeed, got %v", err)
	}
	if len(items) != 1 || len(store.created) != 1 {
		t.Fatalf("expected one imported resource, got items=%#v created=%#v", items, store.created)
	}
	created := store.created[0]
	if created.ApplicationID != "client-id-1" || created.CredentialType != "client_secret" {
		t.Fatalf("expected Graph app metadata and next credential type, got %#v", created)
	}
	if created.CredentialExpiresAt == nil || !created.CredentialExpiresAt.Equal(secretExpiry) {
		t.Fatalf("expected nearest secret expiry to be summarized, got %#v", created.CredentialExpiresAt)
	}
	if created.Status != "expiring" {
		t.Fatalf("expected import status to show incoming expiry, got %q", created.Status)
	}
	if store.snapshotUpdates != 1 || len(store.appCredentials) != 2 || len(store.appOwners) != 1 {
		t.Fatalf("expected credential and owner snapshot, credentials=%#v owners=%#v", store.appCredentials, store.appOwners)
	}
}

func TestSyncAppRegistrationsUpdatesCredentialSnapshot(t *testing.T) {
	secretExpiry := time.Now().UTC().Add(75 * 24 * time.Hour)
	store := &fakeResourceStore{
		managedAppRegs: []Resource{{
			ID:             "res-app",
			Name:           "Old app name",
			Type:           TypeAppRegistration,
			SourceKind:     SourceKindEntraAppRegistration,
			SourceObjectID: "app-object-1",
			ApplicationID:  "client-id-1",
			Owner:          "Alice Admin",
			Secret:         Secret{Mode: SecretModeExternal, Reference: "app-registration://client-id-1"},
		}},
	}
	service := NewService(store, fakeAuditLogger{}, fakeKeyVaultResolver{}, fakeAppRegistrationResolver{
		application: appregistrations.ApplicationItem{
			ID:          "app-object-1",
			AppID:       "client-id-1",
			DisplayName: "Billing Worker",
			Credentials: []appregistrations.CredentialItem{{
				KeyID:       "secret-key-1",
				DisplayName: "prod secret",
				Type:        "client_secret",
				EndDateTime: &secretExpiry,
			}},
		},
	}, nil)

	result, err := service.SyncAppRegistrations(context.Background(), auth.User{ID: "admin", Name: "Admin", IsAdmin: true}, false)
	if err != nil {
		t.Fatalf("expected sync to succeed, got %v", err)
	}
	if result.UpdatedResources != 1 {
		t.Fatalf("expected one app registration update, got %#v", result)
	}
	if len(store.updated) != 1 || store.updated[0].Name != "Billing Worker" {
		t.Fatalf("expected app registration metadata to update, got %#v", store.updated)
	}
	if store.updated[0].CredentialExpiresAt == nil || !store.updated[0].CredentialExpiresAt.Equal(secretExpiry) {
		t.Fatalf("expected credential expiry summary to update, got %#v", store.updated[0].CredentialExpiresAt)
	}
	if store.snapshotUpdates != 1 || len(store.appCredentials) != 1 {
		t.Fatalf("expected credential snapshot refresh, got %#v", store.appCredentials)
	}
}

func TestSyncAppRegistrationsArchivesDeletedApplications(t *testing.T) {
	store := &fakeResourceStore{
		managedAppRegs: []Resource{{
			ID:             "res-app",
			Name:           "Deleted app",
			Type:           TypeAppRegistration,
			SourceKind:     SourceKindEntraAppRegistration,
			SourceObjectID: "app-object-1",
		}},
	}
	service := NewService(store, fakeAuditLogger{}, fakeKeyVaultResolver{}, fakeAppRegistrationResolver{
		err: appregistrations.RequestError{StatusCode: http.StatusNotFound, Body: "not found"},
	}, nil)

	result, err := service.SyncAppRegistrations(context.Background(), auth.User{ID: "admin", Name: "Admin", IsAdmin: true}, false)
	if err != nil {
		t.Fatalf("expected sync to succeed, got %v", err)
	}
	if result.RemovedResources != 1 {
		t.Fatalf("expected one removed resource, got %#v", result)
	}
	if len(store.archive) != 1 || store.archive[0] != "res-app" {
		t.Fatalf("expected deleted app registration to be archived, got %#v", store.archive)
	}
}
