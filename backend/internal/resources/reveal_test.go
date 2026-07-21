package resources

import (
	"context"
	"errors"
	"testing"

	"access-workspace/backend/internal/auth"
)

func TestRevealAllowsPasswordOwnerWithoutRevealRight(t *testing.T) {
	store := &browserExtensionStore{
		items: map[string]Resource{
			"password-1": {
				ID:          "password-1",
				Name:        "Autodesk",
				Type:        TypeWebPortal,
				Category:    "passwords",
				Owner:       "Martin",
				OwnerUserID: "martin",
				Username:    "martin@example.test",
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
		Name:   "Martin",
		Rights: []string{"passwords.edit"},
	}

	result, err := service.Reveal(context.Background(), user, "password-1")
	if err != nil {
		t.Fatalf("expected owner password reveal to succeed, got %v", err)
	}
	if result.SecretValue != "topsecret" {
		t.Fatalf("expected stored password, got %#v", result)
	}
}

func TestRevealRefusesPasswordlessEntry(t *testing.T) {
	store := &browserExtensionStore{
		items: map[string]Resource{
			"password-1": {
				ID:          "password-1",
				Name:        "Claude",
				Type:        TypeWebPortal,
				Category:    "passwords",
				Owner:       "Martin",
				OwnerUserID: "martin",
				Username:    "info@insio.cz",
				Secret: Secret{
					Mode: SecretModeNone,
				},
			},
		},
	}

	service := NewService(store, &captureAuditLogger{}, fakeKeyVaultResolver{}, nil, nil)
	user := auth.User{
		ID:     "martin",
		Name:   "Martin",
		Rights: []string{"passwords.edit"},
	}

	_, err := service.Reveal(context.Background(), user, "password-1")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected passwordless reveal to be refused with invalid input, got %v", err)
	}
}

