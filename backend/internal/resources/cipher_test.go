package resources

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"strings"
	"testing"

	"golang.org/x/crypto/nacl/box"
)

// legacyEncryptV1 produces v1-format ciphertext the way the pre-envelope
// cipher wrote it, so legacy-read compatibility stays covered after the
// production encrypt path moved to v2.
func legacyEncryptV1(t *testing.T, rawKey, value string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(strings.TrimSpace(rawKey)))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		t.Fatalf("legacy cipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("legacy gcm: %v", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		t.Fatalf("legacy nonce: %v", err)
	}
	sealed := gcm.Seal(nonce, nonce, []byte(value), nil)
	return "enc:v1:" + base64.StdEncoding.EncodeToString(sealed)
}

// envelopeRaw seals any value into a v2 envelope, bypassing the idempotence
// check — used to reconstruct corrupted rows (ciphertext inside an envelope)
// that real deployments produced before the healing migration existed.
func envelopeRaw(t *testing.T, c *SecretCipher, value string) string {
	t.Helper()
	dek := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		t.Fatalf("dek: %v", err)
	}
	block, err := aes.NewCipher(dek)
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("gcm: %v", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		t.Fatalf("nonce: %v", err)
	}
	sealedBytes := gcm.Seal(nonce, nonce, []byte(value), nil)
	wrapped, err := c.kek.WrapDEK(context.Background(), dek)
	if err != nil {
		t.Fatalf("wrap: %v", err)
	}
	return "enc:v2:shared:" + c.kek.ID() + ":" +
		base64.StdEncoding.EncodeToString(wrapped) + ":" +
		base64.StdEncoding.EncodeToString(sealedBytes)
}

func TestSecretCipherEncryptDecryptRoundTrip(t *testing.T) {
	ctx := context.Background()
	cipher := NewSecretCipher("test-key")
	encrypted, err := cipher.EncryptForStorage(ctx, "super-secret", SecretClassShared)
	if err != nil {
		t.Fatalf("expected encryption to succeed, got %v", err)
	}
	if encrypted == "super-secret" {
		t.Fatalf("expected encrypted payload to differ from plaintext")
	}
	if !strings.HasPrefix(encrypted, "enc:v2:shared:local:") {
		t.Fatalf("expected v2 envelope with class and kek id, got %q", encrypted)
	}
	decrypted, err := cipher.DecryptFromStorage(ctx, encrypted)
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

func TestSecretCipherUniqueDEKPerSecret(t *testing.T) {
	ctx := context.Background()
	cipher := NewSecretCipher("test-key")
	first, err := cipher.EncryptForStorage(ctx, "same-value", SecretClassShared)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	second, err := cipher.EncryptForStorage(ctx, "same-value", SecretClassShared)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if first == second {
		t.Fatalf("expected distinct envelopes for identical plaintext (fresh DEK per secret)")
	}
}

func TestSecretCipherEncryptIsIdempotent(t *testing.T) {
	ctx := context.Background()
	cipher := NewSecretCipher("test-key")
	encrypted, err := cipher.EncryptForStorage(ctx, "super-secret", SecretClassShared)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	// Update paths round-trip stored ciphertext through the encrypt path;
	// it must come back unchanged, never double-encrypted.
	again, err := cipher.EncryptForStorage(ctx, encrypted, SecretClassShared)
	if err != nil {
		t.Fatalf("re-encrypt: %v", err)
	}
	if again != encrypted {
		t.Fatalf("expected already-encrypted value to pass through unchanged")
	}
	// Same for v1 ciphertext round-tripped by an update.
	legacy := legacyEncryptV1(t, "test-key", "old-secret")
	preserved, err := cipher.EncryptForStorage(ctx, legacy, SecretClassShared)
	if err != nil {
		t.Fatalf("re-encrypt v1: %v", err)
	}
	if preserved != legacy {
		t.Fatalf("expected v1 value to pass through the encrypt path unchanged")
	}
}

func TestSecretCipherReadsLegacyV1(t *testing.T) {
	ctx := context.Background()
	cipher := NewSecretCipher("test-key")
	legacy := legacyEncryptV1(t, "test-key", "old-secret")
	decrypted, err := cipher.DecryptFromStorage(ctx, legacy)
	if err != nil {
		t.Fatalf("expected v1 decryption to succeed, got %v", err)
	}
	if decrypted != "old-secret" {
		t.Fatalf("expected v1 plaintext, got %q", decrypted)
	}
	if !IsEncryptedForStorage(legacy) {
		t.Fatalf("expected v1 value to be recognized as encrypted")
	}
	if !NeedsEncryptionUpgrade(legacy) {
		t.Fatalf("expected v1 value to need upgrade")
	}
}

func TestSecretCipherDecryptFullyHealsNestedLayers(t *testing.T) {
	ctx := context.Background()
	cipher := NewSecretCipher("test-key")

	// v1(v1(p)) — what the old double-encryption bug wrote on every
	// update-without-secret.
	doubleV1 := legacyEncryptV1(t, "test-key", legacyEncryptV1(t, "test-key", "the-password"))
	plain, layers, err := cipher.DecryptFully(ctx, doubleV1)
	if err != nil || plain != "the-password" || layers != 2 {
		t.Fatalf("double v1: got %q layers=%d err=%v", plain, layers, err)
	}

	// v2(v1(p)) — corruption sealed into an envelope. EncryptForStorage
	// refuses to build this (idempotence), so construct it manually.
	innerV1 := legacyEncryptV1(t, "test-key", "the-password")
	sealed := envelopeRaw(t, cipher, innerV1)
	plain, layers, err = cipher.DecryptFully(ctx, sealed)
	if err != nil || plain != "the-password" || layers != 2 {
		t.Fatalf("v2(v1): got %q layers=%d err=%v", plain, layers, err)
	}

	// Healthy single layer and plaintext.
	healthy, err := cipher.EncryptForStorage(ctx, "clean", SecretClassShared)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if plain, layers, err = cipher.DecryptFully(ctx, healthy); err != nil || plain != "clean" || layers != 1 {
		t.Fatalf("healthy: got %q layers=%d err=%v", plain, layers, err)
	}
	if plain, layers, err = cipher.DecryptFully(ctx, "raw-value"); err != nil || plain != "raw-value" || layers != 0 {
		t.Fatalf("plaintext: got %q layers=%d err=%v", plain, layers, err)
	}
}

func TestSecretCipherTamperDetection(t *testing.T) {
	ctx := context.Background()
	cipher := NewSecretCipher("test-key")
	encrypted, err := cipher.EncryptForStorage(ctx, "super-secret", SecretClassShared)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	tampered := encrypted[:len(encrypted)-2] + "AA"
	if tampered == encrypted {
		tampered = encrypted[:len(encrypted)-2] + "BB"
	}
	if _, err := cipher.DecryptFromStorage(ctx, tampered); err == nil {
		t.Fatalf("expected tampered envelope to fail decryption")
	}
}

func TestSecretCipherUnknownKEKProvider(t *testing.T) {
	ctx := context.Background()
	cipher := NewSecretCipher("test-key")
	value := "enc:v2:shared:azure:AAAA:AAAA"
	if _, err := cipher.DecryptFromStorage(ctx, value); err == nil ||
		!strings.Contains(err.Error(), "azure") {
		t.Fatalf("expected unavailable-provider error naming the kek id, got %v", err)
	}
}

func TestSecretCipherWrongKeyFails(t *testing.T) {
	ctx := context.Background()
	encrypted, err := NewSecretCipher("key-one").EncryptForStorage(ctx, "super-secret", SecretClassShared)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := NewSecretCipher("key-two").DecryptFromStorage(ctx, encrypted); err == nil {
		t.Fatalf("expected decryption with a different master key to fail")
	}
}

func TestPersonalEnvelopeRoundTrip(t *testing.T) {
	ctx := context.Background()
	secretCipher := NewSecretCipher("test-key")
	pub, priv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}

	encrypted, err := secretCipher.EncryptPersonalForStorage(ctx, "my-personal-password", pub[:])
	if err != nil {
		t.Fatalf("encrypt personal: %v", err)
	}
	if !strings.HasPrefix(encrypted, "enc:v2:personal:owner:") {
		t.Fatalf("expected personal envelope, got %q", encrypted)
	}
	if !IsPersonalEnvelope(encrypted) || IsPersonalEnvelope("enc:v2:shared:local:AA:BB") {
		t.Fatalf("IsPersonalEnvelope misclassifies")
	}

	// Owner's private key opens it.
	plain, err := secretCipher.DecryptPersonalFromStorage(ctx, encrypted, priv[:])
	if err != nil || plain != "my-personal-password" {
		t.Fatalf("decrypt with owner key: got %q err=%v", plain, err)
	}

	// No key -> vault locked; server-side chain -> vault locked too.
	if _, err := secretCipher.DecryptPersonalFromStorage(ctx, encrypted, nil); !errors.Is(err, ErrVaultLocked) {
		t.Fatalf("expected ErrVaultLocked without key, got %v", err)
	}
	if _, err := secretCipher.DecryptFromStorage(ctx, encrypted); !errors.Is(err, ErrVaultLocked) {
		t.Fatalf("expected ErrVaultLocked via server chain, got %v", err)
	}

	// A different user's key must fail.
	_, otherPriv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate other keypair: %v", err)
	}
	if _, err := secretCipher.DecryptPersonalFromStorage(ctx, encrypted, otherPriv[:]); err == nil {
		t.Fatalf("expected foreign key to fail")
	}

	// Idempotence: round-tripping the ciphertext through either encrypt path
	// leaves it unchanged (update-without-secret regression).
	again, err := secretCipher.EncryptPersonalForStorage(ctx, encrypted, pub[:])
	if err != nil || again != encrypted {
		t.Fatalf("expected personal encrypt idempotence, err=%v", err)
	}
	viaShared, err := secretCipher.EncryptForStorage(ctx, encrypted, SecretClassShared)
	if err != nil || viaShared != encrypted {
		t.Fatalf("expected shared encrypt path to pass personal envelope through, err=%v", err)
	}

	// Non-personal values fall through DecryptPersonalFromStorage.
	shared, err := secretCipher.EncryptForStorage(ctx, "shared-value", SecretClassShared)
	if err != nil {
		t.Fatalf("encrypt shared: %v", err)
	}
	plain, err = secretCipher.DecryptPersonalFromStorage(ctx, shared, priv[:])
	if err != nil || plain != "shared-value" {
		t.Fatalf("expected fall-through for shared value, got %q err=%v", plain, err)
	}
}

func TestIsEncryptedForStoragePlaintext(t *testing.T) {
	ctx := context.Background()
	if IsEncryptedForStorage("plain-value") {
		t.Fatalf("expected plaintext to not be recognized as encrypted")
	}
	if IsEncryptedForStorage("") {
		t.Fatalf("expected empty value to not be recognized as encrypted")
	}
	if !NeedsEncryptionUpgrade("plain-value") {
		t.Fatalf("expected plaintext to need upgrade")
	}
	if NeedsEncryptionUpgrade("") {
		t.Fatalf("expected empty value to not need upgrade")
	}
	// Legacy plaintext rows must survive decryption untouched — the startup
	// migrations rely on this pass-through.
	cipher := NewSecretCipher("test-key")
	passthrough, err := cipher.DecryptFromStorage(ctx, "plain-value")
	if err != nil {
		t.Fatalf("expected plaintext pass-through to succeed, got %v", err)
	}
	if passthrough != "plain-value" {
		t.Fatalf("expected plaintext pass-through to return input, got %q", passthrough)
	}
}
