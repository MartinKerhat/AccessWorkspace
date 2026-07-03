package resources

import (
	"context"
	"testing"

	"access-workspace/backend/internal/audit"
	"access-workspace/backend/internal/auth"
)

type browserExtensionStore struct {
	items map[string]Resource
}

func (s *browserExtensionStore) List(context.Context, Filter) ([]ResourceSummary, error) {
	out := make([]ResourceSummary, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item.Summary())
	}
	return out, nil
}

func (s *browserExtensionStore) Get(_ context.Context, id string) (Resource, error) {
	item, ok := s.items[id]
	if !ok {
		return Resource{}, ErrNotFound
	}
	return item, nil
}

func (s *browserExtensionStore) GetAny(ctx context.Context, id string) (Resource, error) {
	return s.Get(ctx, id)
}

func (s *browserExtensionStore) ListArchived(context.Context) ([]ArchivedResourceSummary, error) {
	return nil, nil
}

func (s *browserExtensionStore) ListManagedKeyVault(context.Context) ([]Resource, error) {
	return nil, nil
}

func (s *browserExtensionStore) ListManagedAppRegistrations(context.Context) ([]Resource, error) {
	return nil, nil
}

func (s *browserExtensionStore) Create(context.Context, CreateResourceInput) (Resource, error) {
	return Resource{}, nil
}

func (s *browserExtensionStore) Update(context.Context, string, UpdateResourceInput) (Resource, error) {
	return Resource{}, nil
}

func (s *browserExtensionStore) Archive(context.Context, string) error { return nil }
func (s *browserExtensionStore) Restore(context.Context, string) error { return nil }
func (s *browserExtensionStore) GetConnectionUserPasswordOverride(context.Context, string, string) (ConnectionCredentialOverride, error) {
	return ConnectionCredentialOverride{}, ErrNotFound
}
func (s *browserExtensionStore) UpsertConnectionUserPasswordOverride(context.Context, string, string, string) error {
	return nil
}
func (s *browserExtensionStore) DeleteConnectionUserPasswordOverride(context.Context, string, string) error {
	return nil
}
func (s *browserExtensionStore) ReplaceAppRegistrationSnapshot(context.Context, string, []AppRegistrationCredential, []AppRegistrationOwner) error {
	return nil
}
func (s *browserExtensionStore) ReplaceAppRegistrationNotificationPolicies(context.Context, string, *AppRegistrationNotificationPolicy, []AppRegistrationCredentialPolicyInput) error {
	return nil
}

func TestListPortalCredentialMatchesUsesURLAndCopyPolicy(t *testing.T) {
	store := &browserExtensionStore{
		items: map[string]Resource{
			"portal-1": {
				ID:          "portal-1",
				Name:        "Autodesk Shared",
				Type:        TypeWebPortal,
				Category:    "passwords",
				Owner:       "Engineering",
				TargetURL:   "https://manage.autodesk.com/apps",
				Username:    "shared.autodesk",
				CopyAllowed: true,
			},
			"portal-2": {
				ID:          "portal-2",
				Name:        "Autodesk Hidden",
				Type:        TypeWebPortal,
				Category:    "passwords",
				Owner:       "Engineering",
				TargetURL:   "https://manage.autodesk.com/apps",
				Username:    "blocked.autodesk",
				CopyAllowed: false,
			},
			"portal-3": {
				ID:          "portal-3",
				Name:        "Grafana Shared",
				Type:        TypeWebPortal,
				Category:    "passwords",
				Owner:       "Platform",
				TargetURL:   "https://grafana.example.test",
				Username:    "shared.grafana",
				CopyAllowed: true,
			},
		},
	}

	service := NewService(store, &captureAuditLogger{}, fakeKeyVaultResolver{}, nil, nil)
	user := auth.User{
		ID:     "martin",
		Rights: []string{"passwords.read"},
	}

	items, err := service.ListPortalCredentialMatches(context.Background(), user, "https://manage.autodesk.com/apps/new")
	if err != nil {
		t.Fatalf("expected portal matches to succeed, got %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one fill-enabled portal match, got %#v", items)
	}
	if items[0].ResourceID != "portal-1" {
		t.Fatalf("expected Autodesk shared match, got %#v", items[0])
	}
}

func TestFillPortalCredentialReturnsSecretAndLogsAudit(t *testing.T) {
	store := &browserExtensionStore{
		items: map[string]Resource{
			"portal-1": {
				ID:          "portal-1",
				Name:        "Autodesk Shared",
				Type:        TypeWebPortal,
				Category:    "passwords",
				Owner:       "Engineering",
				TargetURL:   "https://manage.autodesk.com/apps",
				Username:    "shared.autodesk",
				CopyAllowed: true,
				Secret: Secret{
					Mode:  SecretModeInline,
					Value: "topsecret",
				},
			},
		},
	}
	auditLog := &captureAuditLogger{}
	service := NewService(store, auditLog, fakeKeyVaultResolver{}, nil, nil)
	user := auth.User{
		ID:     "martin",
		Name:   "Martin",
		Rights: []string{"passwords.read"},
	}

	result, err := service.FillPortalCredential(context.Background(), user, "portal-1", "https://manage.autodesk.com/apps/new")
	if err != nil {
		t.Fatalf("expected portal fill to succeed, got %v", err)
	}
	if result.Username != "shared.autodesk" || result.Password != "topsecret" {
		t.Fatalf("expected username and password in fill result, got %#v", result)
	}
	if len(auditLog.entries) != 1 {
		t.Fatalf("expected one audit entry, got %#v", auditLog.entries)
	}
	if auditLog.entries[0].EventType != audit.EventResourceFilled {
		t.Fatalf("expected fill audit event, got %#v", auditLog.entries[0].EventType)
	}
}

func TestFillPortalCredentialRejectsWhenCopyDisabled(t *testing.T) {
	store := &browserExtensionStore{
		items: map[string]Resource{
			"portal-1": {
				ID:        "portal-1",
				Name:      "Autodesk Shared",
				Type:      TypeWebPortal,
				Category:  "passwords",
				Owner:     "Engineering",
				TargetURL: "https://manage.autodesk.com/apps",
				Username:  "shared.autodesk",
				Secret: Secret{
					Mode:  SecretModeInline,
					Value: "topsecret",
				},
			},
		},
	}

	service := NewService(store, &captureAuditLogger{}, fakeKeyVaultResolver{}, nil, nil)
	user := auth.User{
		ID:     "martin",
		Rights: []string{"passwords.read"},
	}

	_, err := service.FillPortalCredential(context.Background(), user, "portal-1", "https://manage.autodesk.com/apps/new")
	if err != ErrForbidden {
		t.Fatalf("expected copy-disabled portal fill to be forbidden, got %v", err)
	}
}
