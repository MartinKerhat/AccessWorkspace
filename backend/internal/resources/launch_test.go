package resources

import (
	"context"
	"testing"
	"time"

	"access-workspace/backend/internal/auth"
)

type launchTestStore struct {
	resource           Resource
	extraResources     map[string]Resource
	overridePasswordID string
}

func (s *launchTestStore) List(context.Context, Filter) ([]ResourceSummary, error) {
	return nil, nil
}

func (s *launchTestStore) Get(_ context.Context, id string) (Resource, error) {
	if s.resource.ID != id {
		if item, ok := s.extraResources[id]; ok {
			return item, nil
		}
		return Resource{}, ErrNotFound
	}
	return s.resource, nil
}

func (s *launchTestStore) GetAny(ctx context.Context, id string) (Resource, error) {
	return s.Get(ctx, id)
}

func (s *launchTestStore) ListArchived(context.Context) ([]ArchivedResourceSummary, error) {
	return nil, nil
}

func (s *launchTestStore) ListManagedKeyVault(context.Context) ([]Resource, error) {
	return nil, nil
}

func (s *launchTestStore) ListManagedAppRegistrations(context.Context) ([]Resource, error) {
	return nil, nil
}

func (s *launchTestStore) Create(context.Context, CreateResourceInput) (Resource, error) {
	return Resource{}, nil
}

func (s *launchTestStore) Update(context.Context, string, UpdateResourceInput) (Resource, error) {
	return Resource{}, nil
}

func (s *launchTestStore) Archive(context.Context, string) error { return nil }
func (s *launchTestStore) Restore(context.Context, string) error { return nil }
func (s *launchTestStore) GetConnectionUserPasswordOverride(_ context.Context, connectionID string, _ string) (ConnectionCredentialOverride, error) {
	if s.overridePasswordID == "" || s.resource.ID != connectionID {
		return ConnectionCredentialOverride{}, ErrNotFound
	}
	return ConnectionCredentialOverride{ConnectionID: connectionID, PasswordResourceID: s.overridePasswordID}, nil
}
func (s *launchTestStore) UpsertConnectionUserPasswordOverride(context.Context, string, string, string) error {
	return nil
}
func (s *launchTestStore) DeleteConnectionUserPasswordOverride(context.Context, string, string) error {
	return nil
}
func (s *launchTestStore) ReplaceAppRegistrationSnapshot(context.Context, string, []AppRegistrationCredential, []AppRegistrationOwner) error {
	return nil
}
func (s *launchTestStore) ReplaceAppRegistrationNotificationPolicies(context.Context, string, *AppRegistrationNotificationPolicy, []AppRegistrationCredentialPolicyInput) error {
	return nil
}

func TestLaunchIssuesLauncherTicketForConnections(t *testing.T) {
	cipher := NewSecretCipher("test-key")
	encryptedSecret, err := cipher.EncryptForStorage(context.Background(), "rdp-password", SecretClassShared)
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}

	store := &launchTestStore{
		resource: Resource{
			ID:                   "conn-1",
			Name:                 "Altron",
			Type:                 TypeRDP,
			Category:             "connections",
			TargetHost:           "91.195.203.250",
			Owner:                "Alice",
			LaunchAllowed:        true,
			Username:             "efminstall",
			ConnectionDomain:     "altron",
			ConnectionScreenMode: "remember_screen",
			Secret: Secret{
				Mode:  SecretModeInline,
				Value: encryptedSecret,
			},
		},
	}

	service := NewService(store, &captureAuditLogger{}, fakeKeyVaultResolver{}, nil, nil, cipher)
	user := auth.User{
		ID:          "alice",
		IsAdmin:     true,
		LocalGroups: []string{"ops-admins"},
		Rights:      []string{"connections.launch"},
	}

	payload, err := service.Launch(context.Background(), user, "conn-1")
	if err != nil {
		t.Fatalf("launch failed: %v", err)
	}

	ticket, ok := payload.Metadata["launcherTicket"].(string)
	if !ok || ticket == "" {
		t.Fatalf("expected launcher ticket in metadata, got %#v", payload.Metadata)
	}
	if payload.Method != "launcher_ticket" {
		t.Fatalf("expected launcher_ticket method, got %q", payload.Method)
	}

	resolved, err := service.ResolveLaunchTicket(context.Background(), ticket)
	if err != nil {
		t.Fatalf("resolve launch ticket failed: %v", err)
	}
	if resolved.Method != "launcher_handoff" {
		t.Fatalf("expected launcher_handoff method, got %q", resolved.Method)
	}
	if metadataString(resolved.Metadata, "secretValue") != "rdp-password" {
		t.Fatalf("expected decrypted secret in resolved payload, got %#v", resolved.Metadata["secretValue"])
	}
	if metadataString(resolved.Metadata, "connectionDomain") != "altron" {
		t.Fatalf("expected connection domain in resolved payload, got %#v", resolved.Metadata["connectionDomain"])
	}
}

func TestLaunchUsesPersonalPasswordOverrideWhenPresent(t *testing.T) {
	cipher := NewSecretCipher("test-key")
	sharedSecret, err := cipher.EncryptForStorage(context.Background(), "shared-password", SecretClassShared)
	if err != nil {
		t.Fatalf("encrypt shared secret: %v", err)
	}
	overrideSecret, err := cipher.EncryptForStorage(context.Background(), "personal-password", SecretClassShared)
	if err != nil {
		t.Fatalf("encrypt override secret: %v", err)
	}

	store := &launchTestStore{
		resource: Resource{
			ID:               "conn-2",
			Name:             "Adele",
			Type:             TypeRDP,
			Category:         "connections",
			TargetHost:       "10.25.60.50",
			LaunchAllowed:    true,
			Username:         "shared-user",
			ConnectionDomain: "intrasoft",
			Secret: Secret{
				Mode:  SecretModeInline,
				Value: sharedSecret,
			},
		},
		overridePasswordID: "pwd-1",
		extraResources: map[string]Resource{
			"pwd-1": {
				ID:          "pwd-1",
				Name:        "Martin Adele Login",
				Type:        TypeSharedSecret,
				Category:    "passwords",
				Personal:    true,
				Owner:       "Martin Kerhat",
				OwnerUserID: "martin",
				Username:    "martin.kerhat",
				SourceKind:  SourceKindManual,
				Secret: Secret{
					Mode:  SecretModeInline,
					Value: overrideSecret,
				},
			},
		},
	}

	service := NewService(store, &captureAuditLogger{}, fakeKeyVaultResolver{}, nil, nil, cipher)
	user := auth.User{
		ID:      "martin",
		Rights:  []string{"connections.read", "passwords.read"},
		IsAdmin: false,
	}

	payload, err := service.Launch(context.Background(), user, "conn-2")
	if err != nil {
		t.Fatalf("launch failed: %v", err)
	}
	ticket, _ := payload.Metadata["launcherTicket"].(string)
	resolved, err := service.ResolveLaunchTicket(context.Background(), ticket)
	if err != nil {
		t.Fatalf("resolve launch ticket failed: %v", err)
	}
	if metadataString(resolved.Metadata, "username") != "martin.kerhat" {
		t.Fatalf("expected override username, got %#v", resolved.Metadata["username"])
	}
	if metadataString(resolved.Metadata, "secretValue") != "personal-password" {
		t.Fatalf("expected override secret, got %#v", resolved.Metadata["secretValue"])
	}
}

func TestLaunchTicketRedeemIsSingleUse(t *testing.T) {
	store := newLaunchTicketStore()
	ticket := store.Issue(LaunchPayload{ResourceID: "conn-1", ResourceType: TypeSSH, Target: "bastion.internal", Metadata: map[string]any{}}, time.Minute)

	if _, err := store.Redeem(ticket); err != nil {
		t.Fatalf("first redeem failed: %v", err)
	}
	if _, err := store.Redeem(ticket); err != ErrNotFound {
		t.Fatalf("expected ticket to be single-use, got %v", err)
	}
}

func metadataString(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return value
}
