package resources

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/MartinKerhat/AccessWorkspace/backend/internal/audit"
	"github.com/MartinKerhat/AccessWorkspace/backend/internal/auth"
	"github.com/MartinKerhat/AccessWorkspace/backend/internal/keyvault"
)

const testSecretRef = "https://vault.example.vault.azure.net/secrets/api-key"

// fakeKeyVaultWriter answers every write with `answer` and records what it
// was asked to write.
type fakeKeyVaultWriter struct {
	fakeKeyVaultResolver
	answer     keyvault.SecretItem
	previous   keyvault.SecretVersion
	versions   []keyvault.SecretVersion
	err        error
	writes     []keyvault.SecretWrite
	references []string
	attributes []keyvault.SecretAttributes
}

func (w *fakeKeyVaultWriter) WritableSources(context.Context) ([]keyvault.Source, error) {
	return []keyvault.Source{{Name: "example", VaultURL: "https://vault.example.vault.azure.net", WriteEnabled: true}}, nil
}

func (w *fakeKeyVaultWriter) CreateSecret(_ context.Context, vaultURL string, name string, input keyvault.SecretWrite) (keyvault.SecretItem, error) {
	w.writes = append(w.writes, input)
	w.references = append(w.references, vaultURL+"/secrets/"+name)
	return w.answer, w.err
}

func (w *fakeKeyVaultWriter) SetSecretValue(_ context.Context, reference string, input keyvault.SecretWrite) (keyvault.SecretItem, error) {
	w.writes = append(w.writes, input)
	w.references = append(w.references, reference)
	return w.answer, w.err
}

func (w *fakeKeyVaultWriter) UpdateSecretAttributes(_ context.Context, reference string, version string, attributes keyvault.SecretAttributes) (keyvault.SecretVersion, keyvault.SecretItem, error) {
	w.attributes = append(w.attributes, attributes)
	w.references = append(w.references, reference+"#"+version)
	return w.previous, w.answer, w.err
}

func (w *fakeKeyVaultWriter) ListSecretVersions(context.Context, string) ([]keyvault.SecretVersion, error) {
	return w.versions, w.err
}

func (w *fakeKeyVaultWriter) RevealSecretVersion(_ context.Context, reference string, version string) (string, error) {
	w.references = append(w.references, reference+"#"+version)
	return "value-of-" + version, w.err
}

// keyVaultRowStore serves one existing catalog row from Get.
type keyVaultRowStore struct {
	fakeResourceStore
	row Resource
}

func (s *keyVaultRowStore) Get(context.Context, string) (Resource, error) {
	return s.row, nil
}

type recordingAudit struct {
	events []audit.LogParams
	past   []audit.Event
}

func (a *recordingAudit) Log(_ context.Context, params audit.LogParams) error {
	a.events = append(a.events, params)
	return nil
}

func (a *recordingAudit) ForResource(context.Context, string, []audit.EventType, int) ([]audit.Event, error) {
	return a.past, nil
}

func keyVaultRow(allowedGroups ...string) Resource {
	return Resource{
		ID:             "kv-1",
		Name:           "api-key",
		Type:           TypeKeyVaultSecret,
		Category:       "keyvault",
		SourceKind:     SourceKindAzureKeyVault,
		SourceObjectID: testSecretRef,
		VaultName:      "example",
		ObjectName:     "api-key",
		Status:         "active",
		OwnerUserID:    "someone-else",
		AllowedGroups:  allowedGroups,
		Secret:         Secret{Mode: SecretModeExternal, Reference: testSecretRef},
	}
}

func userWith(rights ...string) auth.User {
	return auth.User{ID: "u1", Name: "User One", Rights: rights}
}

func TestCreateKeyVaultSecretRegistersCatalogRowWithoutValue(t *testing.T) {
	future := time.Now().UTC().Add(72 * time.Hour)
	writer := &fakeKeyVaultWriter{answer: keyvault.SecretItem{
		ID:        testSecretRef,
		Name:      "api-key",
		VaultName: "example",
		Version:   "v1",
		Enabled:   true,
		NotBefore: &future,
	}}
	store := &fakeResourceStore{}
	auditLog := &recordingAudit{}
	service := NewService(store, auditLog, writer, nil, nil)

	_, err := service.CreateKeyVaultSecret(context.Background(), userWith("keyvault.write"), KeyVaultCreateSecretInput{
		KeyVaultSecretValueInput: KeyVaultSecretValueInput{Value: "s3cr3t-value", Enabled: true, NotBefore: &future, InputMode: "generated"},
		VaultURL:                 "https://vault.example.vault.azure.net",
		SecretName:               "api-key",
		Owner:                    "Somebody Else",
		OwnerUserID:              "someone-else",
		AllowedGroups:            []string{"platform"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(writer.writes) != 1 || writer.writes[0].Value != "s3cr3t-value" {
		t.Fatalf("expected the value to go to Azure once, got %#v", writer.writes)
	}
	if len(store.created) != 1 {
		t.Fatalf("expected one catalog row, got %d", len(store.created))
	}
	row := store.created[0]
	if row.SecretValue != "" || row.SecretMode != SecretModeExternal || row.SecretReference != testSecretRef {
		t.Fatalf("the catalog row must hold only the reference, got mode=%q ref=%q value-set=%v", row.SecretMode, row.SecretReference, row.SecretValue != "")
	}
	if row.OwnerUserID != "u1" || row.Owner != "User One" {
		t.Fatalf("a non-admin creator must own the secret, got %q/%q", row.OwnerUserID, row.Owner)
	}
	if row.Status != "inactive" || row.NotBefore == nil {
		t.Fatalf("expected inactive with the active-from date, got %q %v", row.Status, row.NotBefore)
	}
	if len(auditLog.events) != 1 || auditLog.events[0].EventType != audit.EventKeyVaultSecretCreated {
		t.Fatalf("expected one keyvault_secret_created event, got %#v", auditLog.events)
	}
	if fmt.Sprint(auditLog.events[0].Metadata) == "" || containsValue(auditLog.events[0].Metadata, "s3cr3t-value") {
		t.Fatalf("the audit event must never carry the value: %#v", auditLog.events[0].Metadata)
	}
	if auditLog.events[0].Metadata["inputMode"] != "generated" {
		t.Fatalf("expected the input mode in the audit, got %#v", auditLog.events[0].Metadata)
	}
}

func TestCreateKeyVaultSecretRequiresWriteRight(t *testing.T) {
	writer := &fakeKeyVaultWriter{}
	service := NewService(&fakeResourceStore{}, fakeAuditLogger{}, writer, nil, nil)
	for _, rights := range [][]string{{"keyvault.read"}, {"keyvault.edit"}} {
		_, err := service.CreateKeyVaultSecret(context.Background(), userWith(rights...), KeyVaultCreateSecretInput{
			KeyVaultSecretValueInput: KeyVaultSecretValueInput{Value: "v", Enabled: true},
			VaultURL:                 "https://vault.example.vault.azure.net",
			SecretName:               "x",
		})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("%v: expected forbidden, got %v", rights, err)
		}
	}
	if len(writer.writes) != 0 {
		t.Fatalf("nothing may reach Azure without keyvault.write")
	}
}

func TestCreateKeyVaultSecretSurfacesAzureRefusalWithoutRow(t *testing.T) {
	writer := &fakeKeyVaultWriter{err: keyvault.ErrSecretExists}
	store := &fakeResourceStore{}
	service := NewService(store, fakeAuditLogger{}, writer, nil, nil)
	_, err := service.CreateKeyVaultSecret(context.Background(), userWith("keyvault.write"), KeyVaultCreateSecretInput{
		KeyVaultSecretValueInput: KeyVaultSecretValueInput{Value: "v", Enabled: true},
		VaultURL:                 "https://vault.example.vault.azure.net",
		SecretName:               "api-key",
	})
	if !errors.Is(err, keyvault.ErrSecretExists) {
		t.Fatalf("expected ErrSecretExists, got %v", err)
	}
	if len(store.created) != 0 {
		t.Fatalf("no catalog row for a refused create")
	}
}

func TestSetKeyVaultSecretValueNeedsWriteRightAndAccess(t *testing.T) {
	cases := []struct {
		label  string
		user   auth.User
		row    Resource
		allows bool
	}{
		{"writer, shared with everyone", userWith("keyvault.write"), keyVaultRow(), true},
		{"writer, not in allowed group", userWith("keyvault.write"), keyVaultRow("platform"), false},
		{"edit right only", userWith("keyvault.edit"), keyVaultRow(), false},
		{"admin", auth.User{ID: "admin", IsAdmin: true}, keyVaultRow("platform"), true},
	}
	for _, item := range cases {
		writer := &fakeKeyVaultWriter{answer: keyvault.SecretItem{ID: testSecretRef, Name: "api-key", VaultName: "example", Version: "v2", Enabled: true}}
		store := &keyVaultRowStore{row: item.row}
		service := NewService(store, fakeAuditLogger{}, writer, nil, nil)

		_, err := service.SetKeyVaultSecretValue(context.Background(), item.user, "kv-1", KeyVaultSecretValueInput{Value: "new", Enabled: true})
		if item.allows {
			if err != nil {
				t.Fatalf("%s: expected success, got %v", item.label, err)
			}
			if len(writer.references) != 1 || writer.references[0] != testSecretRef {
				t.Fatalf("%s: expected a write to the stored reference, got %#v", item.label, writer.references)
			}
			if len(store.updated) != 1 || store.updated[0].ObjectVersion != "v2" {
				t.Fatalf("%s: expected the row refreshed with the new version, got %#v", item.label, store.updated)
			}
			continue
		}
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("%s: expected forbidden, got %v", item.label, err)
		}
		if len(writer.writes) != 0 {
			t.Fatalf("%s: nothing may reach Azure", item.label)
		}
	}
}

func TestUpdateKeyVaultSecretVersionStateFollowsEditRightAuditsAndRefreshesRow(t *testing.T) {
	expires := time.Now().UTC().Add(10 * 24 * time.Hour)

	// keyvault.write alone does not cover state (maintainer decision 2026-11-04).
	writer := &fakeKeyVaultWriter{}
	service := NewService(&keyVaultRowStore{row: keyVaultRow()}, fakeAuditLogger{}, writer, nil, nil)
	if _, err := service.UpdateKeyVaultSecretVersionState(context.Background(), userWith("keyvault.write"), "kv-1", "v1", KeyVaultSecretStateInput{Enabled: false}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected forbidden for a write-only user, got %v", err)
	}
	if len(writer.attributes) != 0 {
		t.Fatalf("nothing may reach Azure without the edit right")
	}

	// An older version is disabled; the row follows the current version,
	// which the re-read reports as expiring.
	writer = &fakeKeyVaultWriter{
		answer:   keyvault.SecretItem{ID: testSecretRef, Name: "api-key", VaultName: "example", Version: "v1", Enabled: false},
		previous: keyvault.SecretVersion{Version: "v1", Enabled: true},
	}
	writer.metadata = keyvault.SecretItem{ID: testSecretRef, Name: "api-key", VaultName: "example", Version: "v2", Enabled: true, ExpiresAt: &expires}
	store := &keyVaultRowStore{row: keyVaultRow()}
	auditLog := &recordingAudit{}
	service = NewService(store, auditLog, writer, nil, nil)
	if _, err := service.UpdateKeyVaultSecretVersionState(context.Background(), userWith("keyvault.edit"), "kv-1", "v1", KeyVaultSecretStateInput{Enabled: false}); err != nil {
		t.Fatalf("edit right: %v", err)
	}
	if len(writer.references) != 1 || writer.references[0] != testSecretRef+"#v1" {
		t.Fatalf("expected the change on version v1, got %#v", writer.references)
	}
	if len(store.updated) != 1 || store.updated[0].ObjectVersion != "v2" || store.updated[0].Status != "expiring" {
		t.Fatalf("expected the row refreshed from the current version, got %#v", store.updated)
	}
	if len(auditLog.events) != 1 || auditLog.events[0].EventType != audit.EventKeyVaultSecretStateUpdated {
		t.Fatalf("expected one state event, got %#v", auditLog.events)
	}
	metadata := auditLog.events[0].Metadata
	previous, _ := metadata["previous"].(map[string]any)
	if metadata["version"] != "v1" || metadata["enabled"] != false || previous["enabled"] != true {
		t.Fatalf("expected version and before/after in the audit, got %#v", metadata)
	}
}

func TestListKeyVaultSecretVersionsMarksPinnedAndNamesWorkspaceCreators(t *testing.T) {
	row := keyVaultRow()
	row.Secret.Reference = testSecretRef + "/v1"
	writer := &fakeKeyVaultWriter{versions: []keyvault.SecretVersion{{Version: "v2", Current: true}, {Version: "v1"}}}
	auditLog := &recordingAudit{past: []audit.Event{
		{EventType: audit.EventKeyVaultSecretValueSet, UserName: "Newer Writer", Metadata: map[string]any{"version": "v2"}},
		{EventType: audit.EventKeyVaultSecretCreated, UserName: "First Writer", Metadata: map[string]any{"version": "v1"}},
	}}
	service := NewService(&keyVaultRowStore{row: row}, auditLog, writer, nil, nil)

	if _, err := service.ListKeyVaultSecretVersions(context.Background(), userWith(), "kv-1"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected forbidden without the view right, got %v", err)
	}
	versions, err := service.ListKeyVaultSecretVersions(context.Background(), userWith("keyvault.read"), "kv-1")
	if err != nil {
		t.Fatalf("list versions: %v", err)
	}
	if len(versions) != 2 || versions[0].Pinned || !versions[1].Pinned {
		t.Fatalf("expected only v1 pinned, got %#v", versions)
	}
	if versions[0].CreatedBy != "Newer Writer" || versions[1].CreatedBy != "First Writer" {
		t.Fatalf("expected creators from the audit trail, got %#v", versions)
	}
}

func TestRevealKeyVaultSecretVersionAuditsTheVersion(t *testing.T) {
	writer := &fakeKeyVaultWriter{}
	auditLog := &recordingAudit{}
	service := NewService(&keyVaultRowStore{row: keyVaultRow()}, auditLog, writer, nil, nil)

	// The object's own "copy and reveal allowed" flag governs every version.
	if _, err := service.RevealKeyVaultSecretVersion(context.Background(), userWith("keyvault.read"), "kv-1", "v1"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected forbidden while reveal is not allowed on the object, got %v", err)
	}
	if len(writer.references) != 0 {
		t.Fatalf("a refused reveal must not read from Azure")
	}
	row := keyVaultRow()
	row.RevealAllowed = true
	service = NewService(&keyVaultRowStore{row: row}, auditLog, writer, nil, nil)
	result, err := service.RevealKeyVaultSecretVersion(context.Background(), userWith("keyvault.read"), "kv-1", "v1")
	if err != nil || result.SecretValue != "value-of-v1" {
		t.Fatalf("reveal version: %#v %v", result, err)
	}
	if len(auditLog.events) != 1 || auditLog.events[0].EventType != audit.EventResourceRevealed || auditLog.events[0].Metadata["version"] != "v1" {
		t.Fatalf("expected a reveal event naming the version, got %#v", auditLog.events)
	}
}

func TestGenericEditCannotChangeAzureBackedState(t *testing.T) {
	expires := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	notBefore := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	existing := keyVaultRow()
	existing.Status = "disabled"
	existing.ExpiresAt = &expires
	existing.NotBefore = &notBefore

	input := resourceToUpdateInput(existing)
	input.Status = "active"
	input.ExpiresAt = nil
	input.NotBefore = nil
	input.Description = "changed"

	got := preserveManagedFields(existing, input, auth.User{IsAdmin: true})
	if got.Status != "disabled" || got.ExpiresAt == nil || got.NotBefore == nil {
		t.Fatalf("status and dates of an Azure-backed row must stay as synced, got %q %v %v", got.Status, got.ExpiresAt, got.NotBefore)
	}
	if got.Description != "changed" {
		t.Fatalf("catalog metadata must still be editable")
	}
}

func containsValue(metadata map[string]any, value string) bool {
	for _, item := range metadata {
		if fmt.Sprint(item) == value {
			return true
		}
	}
	return false
}
