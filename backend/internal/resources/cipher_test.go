package resources

import "testing"

func TestSecretCipherEncryptDecryptRoundTrip(t *testing.T) {
	cipher := NewSecretCipher("test-key")
	encrypted, err := cipher.EncryptForStorage("super-secret")
	if err != nil {
		t.Fatalf("expected encryption to succeed, got %v", err)
	}
	if encrypted == "super-secret" {
		t.Fatalf("expected encrypted payload to differ from plaintext")
	}
	decrypted, err := cipher.DecryptFromStorage(encrypted)
	if err != nil {
		t.Fatalf("expected decryption to succeed, got %v", err)
	}
	if decrypted != "super-secret" {
		t.Fatalf("expected decrypted secret to match plaintext, got %q", decrypted)
	}
}
