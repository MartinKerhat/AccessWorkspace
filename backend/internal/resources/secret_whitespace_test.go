package resources

import (
	"context"
	"testing"

	"access-workspace/backend/internal/auth"
)

// A password may legitimately begin or end with whitespace, and a Key Vault
// secret often carries a trailing newline. Trimming the value before encrypting
// it stores a credential that can never authenticate, and because the reveal
// path showed the trimmed value too, the corruption was invisible.
func TestPrepareSecretForStorageKeepsSurroundingWhitespace(t *testing.T) {
	ctx := context.Background()
	cipher := NewSecretCipher("verify-test-key-000000000000000000000000")
	service := &Service{cipher: cipher}
	user := auth.User{ID: "alice"}

	const secret = "  pa ss word\t"
	input := CreateResourceInput{Type: TypeRDP, SecretMode: SecretModeInline, SecretValue: secret}
	if err := service.prepareSecretForStorage(ctx, user, &input); err != nil {
		t.Fatalf("prepare secret for storage: %v", err)
	}
	if !IsEncryptedForStorage(input.SecretValue) {
		t.Fatalf("expected the secret to be encrypted, got %q", input.SecretValue)
	}
	plain, err := cipher.DecryptFromStorage(ctx, input.SecretValue)
	if err != nil {
		t.Fatalf("decrypt stored secret: %v", err)
	}
	if plain != secret {
		t.Fatalf("expected the secret stored byte-for-byte, got %q", plain)
	}
}

// Blank — including whitespace-only — still means "leave the stored secret
// alone", which is what the edit form's "leave blank to keep" relies on.
func TestPrepareSecretForStorageTreatsWhitespaceOnlyAsKeepExisting(t *testing.T) {
	ctx := context.Background()
	service := &Service{cipher: NewSecretCipher("verify-test-key-000000000000000000000000")}

	input := CreateResourceInput{Type: TypeRDP, SecretMode: SecretModeInline, SecretValue: "   "}
	if err := service.prepareSecretForStorage(ctx, auth.User{ID: "alice"}, &input); err != nil {
		t.Fatalf("prepare secret for storage: %v", err)
	}
	if IsEncryptedForStorage(input.SecretValue) {
		t.Fatalf("expected a whitespace-only value not to be stored as a new secret, got %q", input.SecretValue)
	}
}
