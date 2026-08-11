package resources

import (
	"context"
	"errors"
	"testing"
	"time"

	"access-workspace/backend/internal/audit"
	"access-workspace/backend/internal/auth"
)

func TestCreateAllowsSharedPasswordForNonAdminAndForcesOwnership(t *testing.T) {
	store := &personalPasswordStore{}
	service := NewService(store, &captureAuditLogger{}, fakeKeyVaultResolver{}, nil, nil)
	user := auth.User{
		ID:     "martin",
		Name:   "Martin Kerhat",
		Rights: []string{"passwords.create"},
	}

	resource, err := service.Create(context.Background(), user, CreateResourceInput{
		Name:        "Team SQL Login",
		Type:        TypeSharedSecret,
		Personal:    false,
		Owner:       "Somebody Else",
		OwnerUserID: "mallory",
		Username:    "team.sql",
		SecretMode:  SecretModeInline,
		SecretValue: "topsecret",
	})
	if err != nil {
		t.Fatalf("expected shared password create to succeed, got %v", err)
	}
	if resource.Personal {
		t.Fatalf("expected created resource to stay shared")
	}
	if store.createdInput.Owner != "Martin Kerhat" || store.createdInput.OwnerUserID != "martin" {
		t.Fatalf("expected ownership to be forced to creator, got owner=%q ownerUserID=%q", store.createdInput.Owner, store.createdInput.OwnerUserID)
	}
}

// Launch stays meaningless for a saved password. Copy and reveal are one
// permission now, so they must come out of normalization equal to each other
// rather than reveal being cleared (maintainer ruling 2026-08-11).
func TestCreateSharedSecretClearsLaunchAndUnifiesUsageFlags(t *testing.T) {
	store := &personalPasswordStore{}
	service := NewService(store, &captureAuditLogger{}, fakeKeyVaultResolver{}, nil, nil)
	user := auth.User{ID: "martin", Name: "Martin Kerhat", Rights: []string{"passwords.edit"}}

	if _, err := service.Create(context.Background(), user, CreateResourceInput{
		Name:          "Saved Password",
		Type:          TypeSharedSecret,
		Username:      "svc",
		RevealAllowed: true,
		LaunchAllowed: true,
		CopyAllowed:   true,
		SecretMode:    SecretModeInline,
		SecretValue:   "topsecret",
	}); err != nil {
		t.Fatalf("expected create to succeed, got %v", err)
	}
	if store.createdInput.LaunchAllowed {
		t.Fatal("expected the launch flag to be cleared for a saved password")
	}
	if !store.createdInput.RevealAllowed || !store.createdInput.CopyAllowed {
		t.Fatalf("expected copy and reveal to be allowed together, got reveal=%v copy=%v",
			store.createdInput.RevealAllowed, store.createdInput.CopyAllowed)
	}
}

// Setting either half of the pair means the object is usable; normalization
// stores both so nothing ends up half-allowed.
func TestNormalizeUnifiesCopyAndRevealFlags(t *testing.T) {
	copyOnly := normalizeInput(CreateResourceInput{Type: TypeSharedSecret, CopyAllowed: true})
	if !copyOnly.RevealAllowed || !copyOnly.CopyAllowed {
		t.Fatalf("expected copyAllowed to imply revealAllowed, got reveal=%v copy=%v", copyOnly.RevealAllowed, copyOnly.CopyAllowed)
	}

	neither := normalizeInput(CreateResourceInput{Type: TypeSharedSecret})
	if neither.RevealAllowed || neither.CopyAllowed {
		t.Fatalf("expected both flags to stay off, got reveal=%v copy=%v", neither.RevealAllowed, neither.CopyAllowed)
	}

	// A portal with no stored secret has nothing to copy or reveal.
	passwordless := normalizeInput(CreateResourceInput{Type: TypeWebPortal, SecretMode: SecretModeNone, CopyAllowed: true, RevealAllowed: true})
	if passwordless.RevealAllowed || passwordless.CopyAllowed {
		t.Fatalf("expected a passwordless portal to allow neither, got reveal=%v copy=%v", passwordless.RevealAllowed, passwordless.CopyAllowed)
	}
}

func TestOwnerWithoutEditRightCanUpdateAndArchiveOwnedConnection(t *testing.T) {
	store := &archiveTestStore{
		active: Resource{
			ID:          "conn-1",
			Name:        "Created SSH",
			Type:        TypeSSH,
			Category:    CategoryForType(TypeSSH),
			Owner:       "Carl Creator",
			OwnerUserID: "carl",
			TargetHost:  "ssh.example.internal",
			Secret:      Secret{Mode: SecretModePrompt},
		},
	}
	service := NewService(store, &captureAuditLogger{}, fakeKeyVaultResolver{}, nil, nil)
	owner := auth.User{
		ID:     "carl",
		Name:   "Carl Creator",
		Rights: []string{"connections.create"},
	}

	if _, err := service.Update(context.Background(), owner, "conn-1", UpdateResourceInput{
		Name:       "Created SSH",
		Type:       TypeSSH,
		Owner:      "Carl Creator",
		Status:     "active",
		SourceKind: SourceKindManual,
		TargetHost: "ssh.example.internal",
		Username:   "carl",
		SecretMode: SecretModePrompt,
	}); err != nil {
		t.Fatalf("expected create-only owner to update own connection, got %v", err)
	}

	if err := service.Archive(context.Background(), owner, "conn-1"); err != nil {
		t.Fatalf("expected create-only owner to archive own connection, got %v", err)
	}
	if len(store.archivedIDs) != 1 || store.archivedIDs[0] != "conn-1" {
		t.Fatalf("expected shared connection to be archived, got %#v (deleted %#v)", store.archivedIDs, store.deletedIDs)
	}
}

func TestNonAdminCannotReassignOwnerOnUpdate(t *testing.T) {
	store := &personalPasswordStore{
		resource: Resource{
			ID:          "pwd-1",
			Name:        "Team Login",
			Type:        TypeSharedSecret,
			Category:    CategoryForType(TypeSharedSecret),
			Owner:       "Alice",
			OwnerUserID: "alice",
			Username:    "team",
			Secret:      Secret{Mode: SecretModeInline, Value: "existing"},
		},
	}
	service := NewService(store, &captureAuditLogger{}, fakeKeyVaultResolver{}, nil, nil)
	owner := auth.User{ID: "alice", Name: "Alice", Rights: []string{"passwords.edit"}}

	if _, err := service.Update(context.Background(), owner, "pwd-1", UpdateResourceInput{
		Name:        "Team Login",
		Type:        TypeSharedSecret,
		Owner:       "Alice",
		OwnerUserID: "mallory",
		Username:    "team",
		Status:      "active",
		SourceKind:  SourceKindManual,
		SecretMode:  SecretModeInline,
	}); err != nil {
		t.Fatalf("expected owner update to succeed, got %v", err)
	}
	if store.updatedInput.OwnerUserID != "alice" {
		t.Fatalf("expected owner reassignment to be ignored for non-admins, got %q", store.updatedInput.OwnerUserID)
	}
}

func TestAdminCanAssignOwnerAndDefaultsToSelf(t *testing.T) {
	store := &personalPasswordStore{}
	service := NewService(store, &captureAuditLogger{}, fakeKeyVaultResolver{}, nil, nil)
	admin := auth.User{ID: "root", Name: "Root Admin", IsAdmin: true}

	if _, err := service.Create(context.Background(), admin, CreateResourceInput{
		Name:        "Assigned Login",
		Type:        TypeSharedSecret,
		Owner:       "Bob Builder",
		OwnerUserID: "bob",
		Username:    "bob.svc",
		SecretMode:  SecretModeInline,
		SecretValue: "topsecret",
	}); err != nil {
		t.Fatalf("expected admin create to succeed, got %v", err)
	}
	if store.createdInput.OwnerUserID != "bob" {
		t.Fatalf("expected admin-picked owner to be honored, got %q", store.createdInput.OwnerUserID)
	}

	if _, err := service.Create(context.Background(), admin, CreateResourceInput{
		Name:        "Unassigned Login",
		Type:        TypeSharedSecret,
		Username:    "svc",
		SecretMode:  SecretModeInline,
		SecretValue: "topsecret",
	}); err != nil {
		t.Fatalf("expected admin create to succeed, got %v", err)
	}
	if store.createdInput.OwnerUserID != "root" || store.createdInput.Owner != "Root Admin" {
		t.Fatalf("expected ownership to default to the creating admin, got owner=%q ownerUserID=%q", store.createdInput.Owner, store.createdInput.OwnerUserID)
	}
}

func TestArchivePersonalPasswordDeletesPermanently(t *testing.T) {
	store := &archiveTestStore{
		active: Resource{
			ID:          "pwd-1",
			Name:        "My Login",
			Type:        TypeSharedSecret,
			Category:    CategoryForType(TypeSharedSecret),
			Personal:    true,
			Owner:       "Martin",
			OwnerUserID: "martin",
			Username:    "martin.sql",
			Secret:      Secret{Mode: SecretModeInline, Value: "topsecret"},
		},
	}
	logger := &captureAuditLogger{}
	service := NewService(store, logger, fakeKeyVaultResolver{}, nil, nil)
	owner := auth.User{ID: "martin", Name: "Martin", Rights: []string{"passwords.edit"}}

	if err := service.Archive(context.Background(), owner, "pwd-1"); err != nil {
		t.Fatalf("expected personal remove to succeed, got %v", err)
	}
	if len(store.deletedIDs) != 1 || store.deletedIDs[0] != "pwd-1" {
		t.Fatalf("expected personal password to be hard-deleted, got deleted=%#v archived=%#v", store.deletedIDs, store.archivedIDs)
	}
	if len(store.archivedIDs) != 0 {
		t.Fatalf("expected personal password not to be archived, got %#v", store.archivedIDs)
	}
	if len(logger.entries) == 0 || logger.entries[len(logger.entries)-1].EventType != audit.EventResourceDeleted {
		t.Fatalf("expected resource_deleted audit event, got %#v", logger.entries)
	}
}

func TestRestoreRefusesPersonalResources(t *testing.T) {
	archivedAt := time.Now().UTC()
	store := &archiveTestStore{
		archived: Resource{
			ID:          "pwd-legacy",
			Name:        "Legacy Personal",
			Type:        TypeSharedSecret,
			Category:    CategoryForType(TypeSharedSecret),
			Personal:    true,
			OwnerUserID: "martin",
			ArchivedAt:  &archivedAt,
		},
	}
	service := NewService(store, &captureAuditLogger{}, fakeKeyVaultResolver{}, nil, nil)
	admin := auth.User{ID: "root", Name: "Root Admin", IsAdmin: true}

	if err := service.Restore(context.Background(), admin, "pwd-legacy"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected restore of archived personal resource to be hidden (not found), got %v", err)
	}
	if len(store.restoredIDs) != 0 {
		t.Fatalf("expected no restore to happen, got %#v", store.restoredIDs)
	}
}

// A connection whose target forces its own credential prompt is unusable
// unless the people it is shared with can read the password and type it, so
// revealAllowed opens the connection secret to them. Owners and admins never
// need the flag.
func TestRevealConnectionSecretRespectsRevealAllowed(t *testing.T) {
	newStore := func(revealAllowed bool) *browserExtensionStore {
		return &browserExtensionStore{
			items: map[string]Resource{
				"rdp-1": {
					ID:            "rdp-1",
					Name:          "Finance Jump Host",
					Type:          TypeRDP,
					Category:      "connections",
					Owner:         "Alice",
					OwnerUserID:   "alice",
					Username:      "finops-user",
					RevealAllowed: revealAllowed,
					AllowedGroups: []string{"support"},
					Secret:        Secret{Mode: SecretModeInline, Value: "jump-secret"},
				},
			},
		}
	}
	shared := auth.User{ID: "rita", Name: "Rita Reader", Rights: []string{"connections.read"}, LocalGroups: []string{"support"}}
	owner := auth.User{ID: "alice", Name: "Alice", Rights: []string{"connections.read"}}

	service := NewService(newStore(false), &captureAuditLogger{}, fakeKeyVaultResolver{}, nil, nil)
	if _, err := service.Reveal(context.Background(), shared, "rdp-1"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected reveal to be denied for a shared user when revealAllowed is off, got %v", err)
	}
	result, err := service.Reveal(context.Background(), owner, "rdp-1")
	if err != nil {
		t.Fatalf("expected the owner to reveal without the flag, got %v", err)
	}
	if result.SecretValue != "jump-secret" {
		t.Fatalf("expected the stored connection password, got %#v", result)
	}

	service = NewService(newStore(true), &captureAuditLogger{}, fakeKeyVaultResolver{}, nil, nil)
	result, err = service.Reveal(context.Background(), shared, "rdp-1")
	if err != nil {
		t.Fatalf("expected reveal via revealAllowed to succeed for a shared user, got %v", err)
	}
	if result.SecretValue != "jump-secret" {
		t.Fatalf("expected the stored connection password, got %#v", result)
	}
}

// Replaces TestRevealSharedSecretIgnoresRevealAllowedForNonOwner, which asserted
// the old split (revealAllowed alone did NOT grant reveal on a saved password —
// only copyAllowed did). That deny was deliberate under the two-flag model and
// is deliberately gone under the one-flag ruling of 2026-08-11: either half of
// the pair now means the shared user may use the secret. What still matters is
// that a non-owner with NEITHER flag is refused.
func TestRevealSharedSecretRequiresAUsageFlagForNonOwner(t *testing.T) {
	store := &browserExtensionStore{
		items: map[string]Resource{
			"pwd-1": {
				ID:            "pwd-1",
				Name:          "Saved Password",
				Type:          TypeSharedSecret,
				Category:      "passwords",
				Owner:         "Alice",
				OwnerUserID:   "alice",
				Username:      "svc",
				RevealAllowed: false,
				CopyAllowed:   false,
				Secret:        Secret{Mode: SecretModeInline, Value: "topsecret"},
			},
		},
	}
	service := NewService(store, &captureAuditLogger{}, fakeKeyVaultResolver{}, nil, nil)
	reader := auth.User{ID: "rita", Name: "Rita Reader", Rights: []string{"passwords.read"}}

	if _, err := service.Reveal(context.Background(), reader, "pwd-1"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected reveal to be denied with neither usage flag set, got %v", err)
	}

	// Either flag alone is enough — legacy rows may carry only one of them.
	for _, flag := range []string{"reveal", "copy"} {
		item := store.items["pwd-1"]
		item.RevealAllowed = flag == "reveal"
		item.CopyAllowed = flag == "copy"
		store.items["pwd-1"] = item
		result, err := service.Reveal(context.Background(), reader, "pwd-1")
		if err != nil {
			t.Fatalf("expected reveal via %sAllowed to succeed, got %v", flag, err)
		}
		if result.SecretValue != "topsecret" {
			t.Fatalf("expected stored password via %sAllowed, got %#v", flag, result)
		}
	}
}
