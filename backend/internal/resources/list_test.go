package resources

import (
	"context"
	"testing"

	"access-workspace/backend/internal/auth"
)

type listTestStore struct {
	items []ResourceSummary
}

func (s *listTestStore) List(context.Context, Filter) ([]ResourceSummary, error) {
	return s.items, nil
}

func (s *listTestStore) Get(_ context.Context, id string) (Resource, error) {
	for _, item := range s.items {
		if item.ID != id {
			continue
		}
		return Resource{
			ID:            item.ID,
			Name:          item.Name,
			Type:          item.Type,
			Category:      item.Category,
			Status:        item.Status,
			Owner:         item.Owner,
			OwnerTeam:     item.OwnerTeam,
			Environment:   item.Environment,
			VaultName:     item.VaultName,
			ObjectName:    item.ObjectName,
			AllowedGroups: append([]string{}, item.AllowedGroups...),
			RevealAllowed: item.RevealAllowed,
			LaunchAllowed: item.LaunchAllowed,
			Secret: Secret{
				Mode:  SecretModeInline,
				Value: "test-secret",
			},
		}, nil
	}
	return Resource{}, ErrNotFound
}

func (s *listTestStore) GetAny(ctx context.Context, id string) (Resource, error) {
	return s.Get(ctx, id)
}
func (s *listTestStore) ListArchived(context.Context) ([]ArchivedResourceSummary, error) {
	return nil, nil
}
func (s *listTestStore) ListManagedKeyVault(context.Context) ([]Resource, error) { return nil, nil }
func (s *listTestStore) ListManagedAppRegistrations(context.Context) ([]Resource, error) {
	return nil, nil
}
func (s *listTestStore) Create(context.Context, CreateResourceInput) (Resource, error) {
	return Resource{}, nil
}
func (s *listTestStore) Update(context.Context, string, UpdateResourceInput) (Resource, error) {
	return Resource{}, nil
}
func (s *listTestStore) Archive(context.Context, string) error { return nil }
func (s *listTestStore) Restore(context.Context, string) error { return nil }
func (s *listTestStore) GetConnectionUserPasswordOverride(context.Context, string, string) (ConnectionCredentialOverride, error) {
	return ConnectionCredentialOverride{}, ErrNotFound
}
func (s *listTestStore) UpsertConnectionUserPasswordOverride(context.Context, string, string, string) error {
	return nil
}
func (s *listTestStore) DeleteConnectionUserPasswordOverride(context.Context, string, string) error {
	return nil
}
func (s *listTestStore) ReplaceAppRegistrationSnapshot(context.Context, string, []AppRegistrationCredential, []AppRegistrationOwner) error {
	return nil
}
func (s *listTestStore) ReplaceAppRegistrationNotificationPolicies(context.Context, string, *AppRegistrationNotificationPolicy, []AppRegistrationCredentialPolicyInput) error {
	return nil
}

func TestListFiltersByCategoryViewAndAllowedGroups(t *testing.T) {
	store := &listTestStore{
		items: []ResourceSummary{
			{ID: "conn-1", Category: "connections", AllowedGroups: []string{"network"}},
			{ID: "pwd-1", Category: "passwords", AllowedGroups: []string{"network"}},
			{ID: "kv-1", Category: "keyvault", AllowedGroups: []string{"ops-admins"}},
			{ID: "conn-public", Category: "connections", AllowedGroups: nil},
		},
	}
	service := NewService(store, &captureAuditLogger{}, fakeKeyVaultResolver{}, nil, nil)
	user := auth.User{
		ID:          "nina",
		LocalGroups: []string{"network"},
		Rights:      []string{"connections.read"},
	}

	got, err := service.List(context.Background(), user, Filter{})
	if err != nil {
		t.Fatalf("expected list to succeed, got %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected two visible resources, got %#v", got)
	}
	if got[0].ID != "conn-1" || got[1].ID != "conn-public" {
		t.Fatalf("expected only connection resources visible, got %#v", got)
	}
}

func TestExplainVisibleResourcesIncludesVisibilityReasons(t *testing.T) {
	store := &listTestStore{
		items: []ResourceSummary{
			{ID: "conn-public", Name: "Finance Jump Host", Category: "connections", AllowedGroups: nil},
			{ID: "conn-ops", Name: "Ops Jump Host", Category: "connections", AllowedGroups: []string{"network", "ops-admins"}},
			{ID: "pwd-1", Name: "Payroll Secret", Category: "passwords", AllowedGroups: []string{"network"}},
		},
	}
	service := NewService(store, &captureAuditLogger{}, fakeKeyVaultResolver{}, nil, nil)
	user := auth.User{
		ID:          "nina",
		LocalGroups: []string{"network"},
		Rights:      []string{"connections.read"},
	}

	got, err := service.ExplainVisibleResources(context.Background(), user, Filter{})
	if err != nil {
		t.Fatalf("expected visible-resource explanation to succeed, got %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected two visible resources, got %#v", got)
	}
	if got[0].ID != "conn-public" || got[0].VisibilityScope != "everyone" || got[0].CategoryAccessRight != "connections.read" {
		t.Fatalf("expected public connection explanation, got %#v", got[0])
	}
	if got[1].ID != "conn-ops" || got[1].VisibilityScope != "matched_groups" || got[1].CategoryAccessRight != "connections.read" {
		t.Fatalf("expected matched-group connection explanation, got %#v", got[1])
	}
	if len(got[1].MatchedLocalGroups) != 1 || got[1].MatchedLocalGroups[0] != "network" {
		t.Fatalf("expected matched local group to explain access, got %#v", got[1].MatchedLocalGroups)
	}
}

func TestGetRejectsResourcesOutsideCategoryView(t *testing.T) {
	store := &listTestStore{
		items: []ResourceSummary{
			{ID: "kv-1", Name: "otel-api-key", Type: TypeKeyVaultSecret, Category: "keyvault", AllowedGroups: []string{"network"}},
		},
	}
	service := NewService(store, &captureAuditLogger{}, fakeKeyVaultResolver{}, nil, nil)
	user := auth.User{
		ID:          "nina",
		LocalGroups: []string{"network"},
		Rights:      []string{"connections.read"},
	}

	_, err := service.Get(context.Background(), user, "kv-1")
	if err != ErrForbidden {
		t.Fatalf("expected forbidden when category view right is missing, got %v", err)
	}
}

func TestListHidesPersonalPasswordsFromOtherUsers(t *testing.T) {
	store := &listTestStore{
		items: []ResourceSummary{
			{ID: "pwd-personal", Category: "passwords", Type: TypeSharedSecret, Personal: true, Owner: "Alice", OwnerUserID: "alice"},
			{ID: "pwd-shared", Category: "passwords", Type: TypeSharedSecret},
		},
	}
	service := NewService(store, &captureAuditLogger{}, fakeKeyVaultResolver{}, nil, nil)
	user := auth.User{
		ID:     "martin",
		Rights: []string{"passwords.read"},
	}

	got, err := service.List(context.Background(), user, Filter{})
	if err != nil {
		t.Fatalf("expected list to succeed, got %v", err)
	}
	if len(got) != 1 || got[0].ID != "pwd-shared" {
		t.Fatalf("expected only shared password visible, got %#v", got)
	}
}

func TestExplainVisibleResourcesMarksOwnedPersonalPasswordScope(t *testing.T) {
	store := &listTestStore{
		items: []ResourceSummary{
			{ID: "pwd-personal", Name: "Martin SSH", Category: "passwords", Type: TypeSharedSecret, Personal: true, Owner: "Martin", OwnerUserID: "martin"},
		},
	}
	service := NewService(store, &captureAuditLogger{}, fakeKeyVaultResolver{}, nil, nil)
	user := auth.User{
		ID:     "martin",
		Rights: []string{"passwords.edit"},
	}

	got, err := service.ExplainVisibleResources(context.Background(), user, Filter{})
	if err != nil {
		t.Fatalf("expected visible-resource explanation to succeed, got %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected one visible resource, got %#v", got)
	}
	if got[0].VisibilityScope != "personal" {
		t.Fatalf("expected personal visibility scope, got %#v", got[0])
	}
}

func TestListShowsOwnedSharedResourceOutsideAllowedGroups(t *testing.T) {
	store := &listTestStore{
		items: []ResourceSummary{
			{ID: "conn-owned", Name: "Owned SSH", Category: "connections", Type: TypeSSH, Owner: "Martin", OwnerUserID: "martin", AllowedGroups: []string{"ops-admins"}},
		},
	}
	service := NewService(store, &captureAuditLogger{}, fakeKeyVaultResolver{}, nil, nil)
	user := auth.User{
		ID:     "martin",
		Rights: []string{"connections.edit"},
	}

	got, err := service.List(context.Background(), user, Filter{})
	if err != nil {
		t.Fatalf("expected list to succeed, got %v", err)
	}
	if len(got) != 1 || got[0].ID != "conn-owned" {
		t.Fatalf("expected owned shared resource to stay visible, got %#v", got)
	}
}

func TestExplainVisibleResourcesMarksOwnedSharedScope(t *testing.T) {
	store := &listTestStore{
		items: []ResourceSummary{
			{ID: "conn-owned", Name: "Owned SSH", Category: "connections", Type: TypeSSH, Owner: "Martin", OwnerUserID: "martin", AllowedGroups: []string{"ops-admins"}},
		},
	}
	service := NewService(store, &captureAuditLogger{}, fakeKeyVaultResolver{}, nil, nil)
	user := auth.User{
		ID:     "martin",
		Rights: []string{"connections.edit"},
	}

	got, err := service.ExplainVisibleResources(context.Background(), user, Filter{})
	if err != nil {
		t.Fatalf("expected visible-resource explanation to succeed, got %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected one visible resource, got %#v", got)
	}
	if got[0].VisibilityScope != "owner" {
		t.Fatalf("expected owner visibility scope, got %#v", got[0])
	}
}
