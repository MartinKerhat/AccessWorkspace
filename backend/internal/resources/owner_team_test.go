package resources

import (
	"context"
	"errors"
	"testing"

	"github.com/MartinKerhat/AccessWorkspace/backend/internal/auth"
)

// A shared RDP connection owned by alice, with "ops" as the owner team and
// shared with "ops".
func ownerTeamConnection() Resource {
	return Resource{
		ID:            "conn-1",
		Name:          "Ops RDP",
		Type:          TypeRDP,
		Category:      CategoryForType(TypeRDP),
		Owner:         "Alice",
		OwnerUserID:   "alice",
		OwnerTeam:     "ops",
		AllowedGroups: []string{"ops"},
		TargetHost:    "rdp.example.internal",
		Username:      "ops.svc",
		Secret:        Secret{Mode: SecretModePrompt},
	}
}

func ownerTeamUpdateInput() UpdateResourceInput {
	return UpdateResourceInput{
		Name:          "Ops RDP renamed",
		Type:          TypeRDP,
		Owner:         "Alice",
		OwnerUserID:   "mallory",
		OwnerTeam:     "ops",
		AllowedGroups: []string{"ops", "support"},
		Status:        "active",
		SourceKind:    SourceKindManual,
		TargetHost:    "rdp2.example.internal",
		Username:      "ops.svc2",
		SecretMode:    SecretModePrompt,
	}
}

func TestOwnerTeamMemberHasFullEditOnSharedObject(t *testing.T) {
	store := &personalPasswordStore{resource: ownerTeamConnection()}
	service := NewService(store, &captureAuditLogger{}, fakeKeyVaultResolver{}, nil, nil)
	// Read-only on the category: ownership via the team is what grants edit.
	bob := auth.User{ID: "bob", Name: "Bob", LocalGroups: []string{"ops"}, Rights: []string{"connections.read"}}

	if _, err := service.Update(context.Background(), bob, "conn-1", ownerTeamUpdateInput()); err != nil {
		t.Fatalf("expected owner-team member to update the object, got %v", err)
	}
	got := store.updatedInput
	if got.Name != "Ops RDP renamed" || got.TargetHost != "rdp2.example.internal" || got.Username != "ops.svc2" {
		t.Fatalf("expected owner-team member to change owner-level fields, got name=%q host=%q user=%q", got.Name, got.TargetHost, got.Username)
	}
	if len(got.AllowedGroups) != 2 {
		t.Fatalf("expected owner-team member to change sharing, got %#v", got.AllowedGroups)
	}
	// Reassigning the owner user stays admin-only.
	if got.OwnerUserID != "alice" {
		t.Fatalf("expected owner user to stay alice, got %q", got.OwnerUserID)
	}
}

func TestNonOwnerTeamEditorStaysMetadataOnly(t *testing.T) {
	store := &personalPasswordStore{resource: ownerTeamConnection()}
	service := NewService(store, &captureAuditLogger{}, fakeKeyVaultResolver{}, nil, nil)
	// Sees the object (directly shared) and holds the edit right, but is not in the owner team.
	carol := auth.User{ID: "carol", Name: "Carol", LocalGroups: []string{"support"}, Rights: []string{"connections.edit"}}
	existing := ownerTeamConnection()
	existing.AllowedGroups = []string{"ops", "support"}
	store.resource = existing

	if _, err := service.Update(context.Background(), carol, "conn-1", ownerTeamUpdateInput()); err != nil {
		t.Fatalf("expected metadata update to succeed, got %v", err)
	}
	if store.updatedInput.TargetHost != "rdp.example.internal" || store.updatedInput.Name != "Ops RDP" {
		t.Fatalf("expected non-owner edit to be restricted to metadata, got %#v", store.updatedInput)
	}
}

func TestOwnerTeamMemberCannotConvertSharedToPersonal(t *testing.T) {
	store := &personalPasswordStore{resource: ownerTeamConnection()}
	service := NewService(store, &captureAuditLogger{}, fakeKeyVaultResolver{}, nil, nil)
	bob := auth.User{ID: "bob", Name: "Bob", LocalGroups: []string{"ops"}, Rights: []string{"connections.edit"}}

	input := ownerTeamUpdateInput()
	input.Personal = true
	if _, err := service.Update(context.Background(), bob, "conn-1", input); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected shared→personal by an owner-team member to be forbidden, got %v", err)
	}
}

func TestOwnerTeamMemberCanArchiveAndRevealConnectionSecret(t *testing.T) {
	resource := ownerTeamConnection()
	bob := auth.User{ID: "bob", LocalGroups: []string{"ops"}, Rights: []string{"connections.read"}}
	dave := auth.User{ID: "dave", LocalGroups: []string{"ops-readers"}, Rights: []string{"connections.read"}}

	if !canArchiveResource(bob, resource) {
		t.Fatal("expected owner-team member to archive")
	}
	if canArchiveResource(dave, resource) {
		t.Fatal("expected non-member to be refused archive")
	}
	if !canRevealConnectionSecret(bob, resource) {
		t.Fatal("expected owner-team member to reveal the connection secret without a usage flag")
	}
}

func TestOwnerTeamGrantsVisibilityEvenWhenNotInAllowedGroups(t *testing.T) {
	resource := ownerTeamConnection().Summary()
	resource.AllowedGroups = []string{"support"}
	bob := fakeUser{id: "bob", groups: []string{"ops"}}

	if !CanAccess(bob, resource) {
		t.Fatal("expected owner-team member to see the object")
	}
	scope, matched, ok := visibilityScopeForResource(devViewer(auth.User{ID: "bob", LocalGroups: []string{"ops"}}), resource)
	if !ok || scope != "owner_team" || len(matched) != 1 || matched[0] != "ops" {
		t.Fatalf("expected owner_team scope matched on ops, got %q %#v %v", scope, matched, ok)
	}
}

func TestOwnerTeamIgnoredOnPersonalObjects(t *testing.T) {
	resource := ownerTeamConnection()
	resource.Personal = true
	bob := auth.User{ID: "bob", LocalGroups: []string{"ops"}, Rights: []string{"connections.edit"}}

	if CanAccess(devViewer(bob), resource.Summary()) {
		t.Fatal("expected a stale owner team not to expose a personal object")
	}
	if canUpdateResource(bob, resource) || canArchiveResource(bob, resource) {
		t.Fatal("expected a stale owner team not to grant management of a personal object")
	}
}
