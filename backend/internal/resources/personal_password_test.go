package resources

import (
	"context"
	"testing"

	"access-workspace/backend/internal/auth"
)

type personalPasswordStore struct {
	createdInput CreateResourceInput
	updatedInput UpdateResourceInput
	resource     Resource
}

func (s *personalPasswordStore) List(context.Context, Filter) ([]ResourceSummary, error) {
	return nil, nil
}

func (s *personalPasswordStore) Get(context.Context, string) (Resource, error) {
	if s.resource.ID == "" {
		return Resource{}, ErrNotFound
	}
	return s.resource, nil
}

func (s *personalPasswordStore) GetAny(context.Context, string) (Resource, error) {
	return Resource{}, ErrNotFound
}

func (s *personalPasswordStore) ListArchived(context.Context) ([]ArchivedResourceSummary, error) {
	return nil, nil
}

func (s *personalPasswordStore) ListManagedKeyVault(context.Context) ([]Resource, error) {
	return nil, nil
}

func (s *personalPasswordStore) ListManagedAppRegistrations(context.Context) ([]Resource, error) {
	return nil, nil
}

func (s *personalPasswordStore) Create(_ context.Context, input CreateResourceInput) (Resource, error) {
	s.createdInput = input
	return Resource{
		ID:          "pwd-1",
		Name:        input.Name,
		Type:        input.Type,
		Category:    CategoryForType(input.Type),
		Personal:    input.Personal,
		Owner:       input.Owner,
		OwnerUserID: input.OwnerUserID,
		Username:    input.Username,
		Secret: Secret{
			Mode:      input.SecretMode,
			Reference: input.SecretReference,
		},
	}, nil
}

func (s *personalPasswordStore) Update(_ context.Context, _ string, input UpdateResourceInput) (Resource, error) {
	s.updatedInput = input
	s.resource = Resource{
		ID:          "pwd-1",
		Name:        input.Name,
		Type:        input.Type,
		Category:    CategoryForType(input.Type),
		Personal:    input.Personal,
		Owner:       input.Owner,
		OwnerUserID: input.OwnerUserID,
		OwnerTeam:   input.OwnerTeam,
		Username:    input.Username,
		AllowedGroups: append([]string{}, input.AllowedGroups...),
		Secret: Secret{
			Mode:      input.SecretMode,
			Reference: input.SecretReference,
			Value:     input.SecretValue,
		},
	}
	return s.resource, nil
}

func (s *personalPasswordStore) Archive(context.Context, string) error { return nil }
func (s *personalPasswordStore) Restore(context.Context, string) error { return nil }
func (s *personalPasswordStore) GetConnectionUserPasswordOverride(context.Context, string, string) (ConnectionCredentialOverride, error) {
	return ConnectionCredentialOverride{}, ErrNotFound
}
func (s *personalPasswordStore) UpsertConnectionUserPasswordOverride(context.Context, string, string, string) error {
	return nil
}
func (s *personalPasswordStore) DeleteConnectionUserPasswordOverride(context.Context, string, string) error {
	return nil
}
func (s *personalPasswordStore) ReplaceAppRegistrationSnapshot(context.Context, string, []AppRegistrationCredential, []AppRegistrationOwner) error {
	return nil
}
func (s *personalPasswordStore) ReplaceAppRegistrationNotificationPolicies(context.Context, string, *AppRegistrationNotificationPolicy, []AppRegistrationCredentialPolicyInput) error {
	return nil
}

func TestCreateAllowsPersonalPasswordForNonAdmin(t *testing.T) {
	store := &personalPasswordStore{}
	service := NewService(store, &captureAuditLogger{}, fakeKeyVaultResolver{}, nil, nil)
	user := auth.User{
		ID:     "martin",
		Name:   "Martin Kerhat",
		Rights: []string{"passwords.edit"},
	}

	resource, err := service.Create(context.Background(), user, CreateResourceInput{
		Name:        "Martin SQL Login",
		Type:        TypeSharedSecret,
		Personal:    true,
		Username:    "martin.sql",
		SecretMode:  SecretModeInline,
		SecretValue: "topsecret",
	})
	if err != nil {
		t.Fatalf("expected personal password create to succeed, got %v", err)
	}
	if !resource.Personal {
		t.Fatalf("expected created resource to stay personal")
	}
	if store.createdInput.Owner != "Martin Kerhat" || store.createdInput.OwnerUserID != "martin" {
		t.Fatalf("expected ownership to be forced to current user, got owner=%q ownerUserID=%q", store.createdInput.Owner, store.createdInput.OwnerUserID)
	}
	if len(store.createdInput.AllowedGroups) != 0 {
		t.Fatalf("expected personal password allowed groups to be cleared, got %#v", store.createdInput.AllowedGroups)
	}
}

func TestAdminCannotManageAnotherUsersPersonalPassword(t *testing.T) {
	store := &personalPasswordStore{
		resource: Resource{
			ID:          "pwd-1",
			Name:        "Grafana",
			Type:        TypeWebPortal,
			Category:    CategoryForType(TypeWebPortal),
			Personal:    true,
			Owner:       "Alice Admin",
			OwnerUserID: "alice",
			Username:    "admin",
			TargetURL:   "https://grafana.example/login",
			Secret: Secret{
				Mode:  SecretModeInline,
				Value: "existing-secret",
			},
		},
	}
	service := NewService(store, &captureAuditLogger{}, fakeKeyVaultResolver{}, nil, nil)
	admin := auth.User{
		ID:      "martin",
		Name:    "Martin Kerhat",
		IsAdmin: true,
	}

	// An admin who does not own the personal password must not be able to update
	// it — otherwise they could flip it to shared and then reveal the secret.
	if _, err := service.Update(context.Background(), admin, "pwd-1", UpdateResourceInput{
		Name:        "Grafana",
		Type:        TypeWebPortal,
		Personal:    false,
		Owner:       "Alice Admin",
		OwnerTeam:   "ops-admins",
		Status:      "active",
		SourceKind:  SourceKindManual,
		TargetURL:   "https://grafana.example/login",
		Username:    "admin",
		CopyAllowed: true,
		SecretMode:  SecretModeInline,
	}); err != ErrForbidden {
		t.Fatalf("expected admin update of another user's personal password to be forbidden, got %v", err)
	}

	// An admin must not be able to reveal another user's personal password.
	if _, err := service.Reveal(context.Background(), admin, "pwd-1"); err != ErrForbidden {
		t.Fatalf("expected admin reveal of another user's personal password to be forbidden, got %v", err)
	}

	// An admin must not be able to view another user's personal password.
	if _, err := service.Get(context.Background(), admin, "pwd-1"); err != ErrForbidden {
		t.Fatalf("expected admin view of another user's personal password to be forbidden, got %v", err)
	}
}

func TestOwnerCanConvertPersonalWebPortalPasswordToShared(t *testing.T) {
	store := &personalPasswordStore{
		resource: Resource{
			ID:          "pwd-1",
			Name:        "Grafana",
			Type:        TypeWebPortal,
			Category:    CategoryForType(TypeWebPortal),
			Personal:    true,
			Owner:       "Alice Admin",
			OwnerUserID: "alice",
			Username:    "admin",
			TargetURL:   "https://grafana.example/login",
			Secret: Secret{
				Mode:  SecretModeInline,
				Value: "existing-secret",
			},
		},
	}
	service := NewService(store, &captureAuditLogger{}, fakeKeyVaultResolver{}, nil, nil)
	owner := auth.User{
		ID:     "alice",
		Name:   "Alice Admin",
		Rights: []string{"passwords.edit"},
	}

	updated, err := service.Update(context.Background(), owner, "pwd-1", UpdateResourceInput{
		Name:          "Grafana",
		Type:          TypeWebPortal,
		Personal:      false,
		Description:   "Shared portal login",
		Owner:         "Alice Admin",
		OwnerTeam:     "ops-admins",
		Status:        "active",
		SourceKind:    SourceKindManual,
		TargetURL:     "https://grafana.example/login",
		Username:      "admin",
		CopyAllowed:   true,
		AllowedGroups: []string{"ops-admins"},
		SecretMode:    SecretModeInline,
	})
	if err != nil {
		t.Fatalf("expected owner update to succeed, got %v", err)
	}
	if updated.Personal {
		t.Fatalf("expected updated resource to become shared")
	}
	if store.updatedInput.OwnerUserID != "alice" {
		t.Fatalf("expected owner user id to be preserved, got %q", store.updatedInput.OwnerUserID)
	}
}

func TestOwnerCanUpdateOwnedConnection(t *testing.T) {
	store := &personalPasswordStore{
		resource: Resource{
			ID:          "conn-1",
			Name:        "Owned SSH",
			Type:        TypeSSH,
			Category:    CategoryForType(TypeSSH),
			Owner:       "Alice Admin",
			OwnerUserID: "alice",
			TargetHost:  "ssh.example.internal",
			Username:    "alice",
			Secret: Secret{
				Mode:  SecretModePrompt,
				Value: "",
			},
		},
	}
	service := NewService(store, &captureAuditLogger{}, fakeKeyVaultResolver{}, nil, nil)
	owner := auth.User{
		ID:     "alice",
		Name:   "Alice Admin",
		Rights: []string{"connections.edit"},
	}

	updated, err := service.Update(context.Background(), owner, "conn-1", UpdateResourceInput{
		Name:          "Owned SSH",
		Type:          TypeSSH,
		Owner:         "Alice Admin",
		Status:        "active",
		SourceKind:    SourceKindManual,
		TargetHost:    "ssh.example.internal",
		Username:      "alice.updated",
		LaunchMode:    "native_launcher",
		LaunchAllowed: true,
		SecretMode:    SecretModePrompt,
	})
	if err != nil {
		t.Fatalf("expected owner connection update to succeed, got %v", err)
	}
	if updated.Type != TypeSSH || updated.Name != "Owned SSH" {
		t.Fatalf("expected updated owned connection, got %#v", updated)
	}
}
