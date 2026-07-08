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
	if !IsEncryptedForStorage(encrypted) {
		t.Fatalf("expected encrypted payload to be recognized as encrypted")
	}
}

func TestIsEncryptedForStoragePlaintext(t *testing.T) {
	if IsEncryptedForStorage("plain-value") {
		t.Fatalf("expected plaintext to not be recognized as encrypted")
	}
	if IsEncryptedForStorage("") {
		t.Fatalf("expected empty value to not be recognized as encrypted")
	}
	// Legacy plaintext rows must survive decryption untouched — the settings
	// migration relies on this pass-through.
	cipher := NewSecretCipher("test-key")
	passthrough, err := cipher.DecryptFromStorage("plain-value")
	if err != nil {
		t.Fatalf("expected plaintext pass-through to succeed, got %v", err)
	}
	if passthrough != "plain-value" {
		t.Fatalf("expected plaintext pass-through to return input, got %q", passthrough)
	}
}
