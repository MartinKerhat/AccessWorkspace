package resources

import (
	"context"
	"errors"
	"testing"

	"github.com/MartinKerhat/AccessWorkspace/backend/internal/auth"
)

func TestImportKeyVaultSecretsSkipsAlreadyManagedAndBatchDuplicates(t *testing.T) {
	store := &fakeResourceStore{
		managed: []Resource{
			{
				ID:             "res-1",
				Name:           "existing-secret",
				Type:           TypeKeyVaultSecret,
				SourceKind:     SourceKindAzureKeyVault,
				SourceObjectID: "https://vault.example.vault.azure.net/secrets/existing-secret",
				VaultName:      "example",
				ObjectName:     "existing-secret",
				Secret: Secret{
					Mode:      SecretModeExternal,
					Reference: "https://vault.example.vault.azure.net/secrets/existing-secret",
				},
			},
		},
	}
	service := NewService(store, fakeAuditLogger{}, fakeKeyVaultResolver{}, nil, nil)

	disabled := false
	result, err := service.ImportKeyVaultSecrets(context.Background(), auth.User{ID: "admin", Name: "Admin", IsAdmin: true}, KeyVaultImportInput{
		Owner:         "Alice Admin",
		OwnerTeam:     "platform",
		Environment:   "prod",
		AllowedGroups: []string{"platform"},
		Items: []KeyVaultImportItem{
			{
				// Same secret as the managed row, differing only by a trailing
				// slash: the auto-import path normalizes this away, so manual
				// import has to as well.
				VaultURL:   "https://vault.example.vault.azure.net",
				VaultName:  "example",
				ObjectName: "existing-secret",
				SecretID:   "https://vault.example.vault.azure.net/secrets/existing-secret/",
			},
			{
				VaultURL:    "https://vault.example.vault.azure.net",
				VaultName:   "example",
				ObjectName:  "new-secret",
				SecretID:    "https://vault.example.vault.azure.net/secrets/new-secret",
				ContentType: "text/plain",
				Enabled:     &disabled,
			},
			{
				// Selected twice in one request.
				VaultURL:   "https://vault.example.vault.azure.net",
				VaultName:  "example",
				ObjectName: "new-secret",
				SecretID:   "https://vault.example.vault.azure.net/secrets/new-secret",
			},
		},
	})
	if err != nil {
		t.Fatalf("expected import to succeed, got %v", err)
	}
	if len(result.Items) != 1 || len(store.created) != 1 {
		t.Fatalf("expected exactly one new resource, got items=%d created=%#v", len(result.Items), store.created)
	}
	if result.Skipped != 2 {
		t.Fatalf("expected the managed duplicate and the batch duplicate to be skipped, got %d", result.Skipped)
	}
	created := store.created[0]
	if created.SourceObjectID != "https://vault.example.vault.azure.net/secrets/new-secret" {
		t.Fatalf("expected normalized secret id as source object id, got %q", created.SourceObjectID)
	}
	if created.SecretReference != created.SourceObjectID || created.LinkedSecretRef != created.SourceObjectID {
		t.Fatalf("expected secret reference and linked ref to match the secret id, got %#v", created)
	}
	if created.Status != "disabled" || created.Owner != "Alice Admin" || created.VaultName != "example" || created.ObjectName != "new-secret" {
		t.Fatalf("expected import settings to be applied, got %#v", created)
	}
}

func TestImportKeyVaultSecretsRejectsNonAdmin(t *testing.T) {
	store := &fakeResourceStore{}
	service := NewService(store, fakeAuditLogger{}, fakeKeyVaultResolver{}, nil, nil)

	_, err := service.ImportKeyVaultSecrets(context.Background(), auth.User{ID: "editor", Name: "Editor"}, KeyVaultImportInput{
		Owner: "Editor",
		Items: []KeyVaultImportItem{{SecretID: "https://vault.example.vault.azure.net/secrets/a", ObjectName: "a", VaultName: "example"}},
	})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
	if len(store.created) != 0 {
		t.Fatalf("expected nothing created, got %#v", store.created)
	}
}
