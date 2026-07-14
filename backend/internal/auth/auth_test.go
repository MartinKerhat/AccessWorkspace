package auth

import (
	"context"
	"testing"
)

func TestCapabilitiesForUserUsesGroups(t *testing.T) {
	capabilities := CapabilitiesForUser(User{
		ID:     "sam",
		Name:   "Sam Support",
		Rights: []string{"connections.read", "passwords.read"},
	})

	if !capabilities.Categories["connections"].View {
		t.Fatalf("expected support group to view connections")
	}
	if capabilities.Categories["keyvault"].View {
		t.Fatalf("expected support group not to view key vault")
	}
	if capabilities.CanViewAudit {
		t.Fatalf("expected non-admin user not to view audit")
	}
}

func TestCapabilitiesForAdminEnableAdminAreas(t *testing.T) {
	capabilities := CapabilitiesForUser(User{
		ID:      "alice",
		Name:    "Alice Admin",
		Rights:  []string{"admin.access"},
		IsAdmin: true,
	})

	if !capabilities.CanViewAudit {
		t.Fatalf("expected admin user to view audit")
	}
	if !capabilities.CanViewAdmin {
		t.Fatalf("expected admin user to view admin")
	}
	if !capabilities.Categories["keyvault"].View {
		t.Fatalf("expected ops-admins admin to view key vault")
	}
}

func TestCapabilitiesForPasswordsCreateRight(t *testing.T) {
	capabilities := CapabilitiesForUser(User{
		ID:     "carl",
		Name:   "Carl Creator",
		Rights: []string{"passwords.create"},
	})

	passwords := capabilities.Categories["passwords"]
	if !passwords.View || !passwords.Create || !passwords.Reveal || !passwords.Launch {
		t.Fatalf("expected passwords.create to imply read-level access plus create, got %#v", passwords)
	}
	if passwords.Edit {
		t.Fatalf("expected passwords.create not to grant edit")
	}
}

func TestCapabilitiesImportIsAdminOnly(t *testing.T) {
	capabilities := CapabilitiesForUser(User{
		ID:     "kim",
		Name:   "Kim Keyvault",
		Rights: []string{"keyvault.edit", "appregistrations.edit"},
	})

	if capabilities.Categories["keyvault"].Import {
		t.Fatalf("expected keyvault import to stay admin-only")
	}
	if capabilities.Categories["appregistrations"].Import {
		t.Fatalf("expected app registration import to stay admin-only")
	}
}

func TestCreateUserValidation(t *testing.T) {
	service := &Service{}

	cases := []CreateUserInput{
		{DisplayName: "Test", Email: "test@example.internal", Password: "password123"},
		{Username: "test", Email: "test@example.internal", Password: "password123"},
		{Username: "test", DisplayName: "Test", Password: "password123"},
		{Username: "test", DisplayName: "Test", Email: "invalid-email", Password: "password123"},
		{Username: "test", DisplayName: "Test", Email: "test@example.internal", Password: "short"},
	}

	for _, input := range cases {
		if _, err := service.CreateUser(context.Background(), input); err == nil {
			t.Fatalf("expected validation error for %#v", input)
		}
	}
}
