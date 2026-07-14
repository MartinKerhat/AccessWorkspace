package resources

import (
	"context"
	"testing"
	"time"

	"access-workspace/backend/internal/audit"
	"access-workspace/backend/internal/auth"
)

type archiveTestStore struct {
	active        Resource
	archived      Resource
	archivedItems []ArchivedResourceSummary
	archivedIDs   []string
	deletedIDs    []string
	restoredIDs   []string
}

func (s *archiveTestStore) List(context.Context, Filter) ([]ResourceSummary, error) {
	return nil, nil
}

func (s *archiveTestStore) Get(_ context.Context, id string) (Resource, error) {
	if s.active.ID == id {
		return s.active, nil
	}
	return Resource{}, ErrNotFound
}

func (s *archiveTestStore) GetAny(_ context.Context, id string) (Resource, error) {
	switch id {
	case s.active.ID:
		return s.active, nil
	case s.archived.ID:
		return s.archived, nil
	default:
		return Resource{}, ErrNotFound
	}
}

func (s *archiveTestStore) ListArchived(context.Context) ([]ArchivedResourceSummary, error) {
	return s.archivedItems, nil
}

func (s *archiveTestStore) ListManagedKeyVault(context.Context) ([]Resource, error) {
	return nil, nil
}

func (s *archiveTestStore) ListManagedAppRegistrations(context.Context) ([]Resource, error) {
	return nil, nil
}

func (s *archiveTestStore) Create(context.Context, CreateResourceInput) (Resource, error) {
	return Resource{}, nil
}

func (s *archiveTestStore) Update(context.Context, string, UpdateResourceInput) (Resource, error) {
	return Resource{}, nil
}

func (s *archiveTestStore) Archive(_ context.Context, id string) error {
	s.archivedIDs = append(s.archivedIDs, id)
	return nil
}

func (s *archiveTestStore) Delete(_ context.Context, id string) error {
	s.deletedIDs = append(s.deletedIDs, id)
	return nil
}

func (s *archiveTestStore) Restore(_ context.Context, id string) error {
	s.restoredIDs = append(s.restoredIDs, id)
	return nil
}

func (s *archiveTestStore) GetConnectionUserPasswordOverride(context.Context, string, string) (ConnectionCredentialOverride, error) {
	return ConnectionCredentialOverride{}, ErrNotFound
}

func (s *archiveTestStore) UpsertConnectionUserPasswordOverride(context.Context, string, string, string) error {
	return nil
}

func (s *archiveTestStore) DeleteConnectionUserPasswordOverride(context.Context, string, string) error {
	return nil
}

func (s *archiveTestStore) ReplaceAppRegistrationSnapshot(context.Context, string, []AppRegistrationCredential, []AppRegistrationOwner) error {
	return nil
}

func (s *archiveTestStore) ReplaceAppRegistrationNotificationPolicies(context.Context, string, *AppRegistrationNotificationPolicy, []AppRegistrationCredentialPolicyInput) error {
	return nil
}

type captureAuditLogger struct {
	entries []audit.LogParams
}

func (l *captureAuditLogger) Log(_ context.Context, params audit.LogParams) error {
	l.entries = append(l.entries, params)
	return nil
}

func TestListArchivedRequiresAdmin(t *testing.T) {
	service := NewService(&archiveTestStore{}, &captureAuditLogger{}, fakeKeyVaultResolver{}, nil, nil)

	_, err := service.ListArchived(context.Background(), auth.User{ID: "member"})
	if err != ErrForbidden {
		t.Fatalf("expected forbidden for member user, got %v", err)
	}
}

func TestArchiveLogsRemovedFromAppReason(t *testing.T) {
	store := &archiveTestStore{
		active: Resource{
			ID:   "res-1",
			Name: "Key vault record",
			Type: TypeKeyVaultSecret,
		},
	}
	auditLog := &captureAuditLogger{}
	service := NewService(store, auditLog, fakeKeyVaultResolver{}, nil, nil)

	err := service.Archive(context.Background(), auth.User{ID: "admin", Name: "Admin", IsAdmin: true}, "res-1")
	if err != nil {
		t.Fatalf("expected archive to succeed, got %v", err)
	}
	if len(store.archivedIDs) != 1 || store.archivedIDs[0] != "res-1" {
		t.Fatalf("expected resource to be archived, got %#v", store.archivedIDs)
	}
	if len(auditLog.entries) != 1 {
		t.Fatalf("expected one audit entry, got %#v", auditLog.entries)
	}
	if auditLog.entries[0].EventType != audit.EventResourceArchived {
		t.Fatalf("expected archive audit event, got %#v", auditLog.entries[0].EventType)
	}
	if got := auditLog.entries[0].Metadata["reason"]; got != "removed_from_app" {
		t.Fatalf("expected removed_from_app reason, got %#v", got)
	}
}

func TestRestoreArchivedResourceLogsAudit(t *testing.T) {
	archivedAt := time.Now().UTC().Add(-time.Hour)
	store := &archiveTestStore{
		archived: Resource{
			ID:         "res-2",
			Name:       "Archived object",
			Type:       TypeSharedSecret,
			ArchivedAt: &archivedAt,
		},
	}
	auditLog := &captureAuditLogger{}
	service := NewService(store, auditLog, fakeKeyVaultResolver{}, nil, nil)

	err := service.Restore(context.Background(), auth.User{ID: "admin", Name: "Admin", IsAdmin: true}, "res-2")
	if err != nil {
		t.Fatalf("expected restore to succeed, got %v", err)
	}
	if len(store.restoredIDs) != 1 || store.restoredIDs[0] != "res-2" {
		t.Fatalf("expected restore call, got %#v", store.restoredIDs)
	}
	if len(auditLog.entries) != 1 {
		t.Fatalf("expected one audit entry, got %#v", auditLog.entries)
	}
	if auditLog.entries[0].EventType != audit.EventResourceRestored {
		t.Fatalf("expected restored audit event, got %#v", auditLog.entries[0].EventType)
	}
}
