package resources

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Storage formats, oldest first:
//
//	plaintext                                      pre-encryption legacy, read-only
//	enc:v1:<b64(nonce||gcm(value))>                single master key, read-only
//	enc:v2:<class>:<kek-id>:<b64(wrapped-dek)>:<b64(nonce||gcm(value))>
//
// v2 is envelope encryption: every secret gets its own random 32-byte DEK,
// the value is sealed with the DEK, and the DEK is wrapped by a KEKProvider.
// The format is self-contained (no schema columns needed), so it works for
// resource_secrets.secret_value, admin_settings.value, and any future column.
// Class and kek-id must not contain ':'.
const (
	legacySecretPrefix = "enc:v1:"
	envelopePrefix     = "enc:v2:"
)

// SecretClass records why a secret is recoverable and by whom. shared and
// app-scope both wrap with the org KEK today; personal (later phase) will
// wrap with a per-owner key, which is why the class is stored per secret.
type SecretClass string

const (
	SecretClassShared   SecretClass = "shared"
	SecretClassAppScope SecretClass = "app"
)

type SecretCipher struct {
	legacy cipher.AEAD            // v1 reads only
	kek    KEKProvider            // wraps new DEKs
	unwrap map[string]KEKProvider // by ID; superset of kek during rotation
}

func NewSecretCipher(rawKey string) *SecretCipher {
	sum := sha256.Sum256([]byte(strings.TrimSpace(rawKey)))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		panic(fmt.Sprintf("secret cipher: %v", err)) // key is sha256-sized; cannot fail
	}
	legacy, err := cipher.NewGCM(block)
	if err != nil {
		panic(fmt.Sprintf("secret cipher: %v", err))
	}
	kek, err := newLocalKEKProvider(rawKey)
	if err != nil {
		panic(fmt.Sprintf("secret cipher: %v", err))
	}
	return &SecretCipher{
		legacy: legacy,
		kek:    kek,
		unwrap: map[string]KEKProvider{kek.ID(): kek},
	}
}

// IsEncryptedForStorage reports whether the value already carries one of the
// storage-encryption envelopes.
func IsEncryptedForStorage(value string) bool {
	trimmed := strings.TrimSpace(value)
	return strings.HasPrefix(trimmed, envelopePrefix) || strings.HasPrefix(trimmed, legacySecretPrefix)
}

// NeedsEncryptionUpgrade reports whether a stored value should be re-written
// as a v2 envelope: pre-encryption plaintext and v1 ciphertext both qualify.
func NeedsEncryptionUpgrade(value string) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed != "" && !strings.HasPrefix(trimmed, envelopePrefix)
}

// EncryptForStorage seals the value into a v2 envelope. Values that already
// carry an envelope are returned unchanged: encryption is idempotent, so
// update paths that round-trip stored ciphertext cannot double-encrypt.
func (c *SecretCipher) EncryptForStorage(ctx context.Context, value string, class SecretClass) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	if IsEncryptedForStorage(trimmed) {
		return trimmed, nil
	}
	if strings.Contains(string(class), ":") || class == "" {
		return "", fmt.Errorf("invalid secret class %q", class)
	}

	dek := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return "", err
	}
	block, err := aes.NewCipher(dek)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(trimmed), nil)

	wrapped, err := c.kek.WrapDEK(ctx, dek)
	if err != nil {
		return "", err
	}
	for i := range dek {
		dek[i] = 0
	}

	return envelopePrefix + string(class) + ":" + c.kek.ID() + ":" +
		base64.StdEncoding.EncodeToString(wrapped) + ":" +
		base64.StdEncoding.EncodeToString(sealed), nil
}

// DecryptFully unwraps every encryption layer until plaintext remains and
// reports how many layers were removed. Healthy values have exactly one
// layer; more mean the value was corrupted by the historical
// double-encryption bug (updates that round-tripped stored ciphertext through
// the encrypt path before it was idempotent). The startup migrations use this
// to fully heal such rows.
func (c *SecretCipher) DecryptFully(ctx context.Context, value string) (string, int, error) {
	const maxLayers = 32 // far beyond any real corruption; guards the loop
	layers := 0
	current := strings.TrimSpace(value)
	for IsEncryptedForStorage(current) {
		if layers >= maxLayers {
			return "", layers, errors.New("secret exceeds maximum encryption nesting")
		}
		plain, err := c.DecryptFromStorage(ctx, current)
		if err != nil {
			if layers > 0 {
				// The nested value only looks encrypted (a plaintext that
				// happens to carry the prefix); keep it as the final value.
				return current, layers, nil
			}
			return "", layers, err
		}
		current = strings.TrimSpace(plain)
		layers++
	}
	return current, layers, nil
}

// DecryptFromStorage reads any storage format: v2 envelopes, v1 legacy
// ciphertext, and pre-encryption plaintext (returned as-is).
func (c *SecretCipher) DecryptFromStorage(ctx context.Context, value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	if strings.HasPrefix(trimmed, envelopePrefix) {
		return c.openEnvelope(ctx, strings.TrimPrefix(trimmed, envelopePrefix))
	}
	if strings.HasPrefix(trimmed, legacySecretPrefix) {
		return c.openLegacy(strings.TrimPrefix(trimmed, legacySecretPrefix))
	}
	return trimmed, nil
}

func (c *SecretCipher) openEnvelope(ctx context.Context, payload string) (string, error) {
	parts := strings.SplitN(payload, ":", 4)
	if len(parts) != 4 {
		return "", errors.New("encrypted secret envelope is invalid")
	}
	kekID := parts[1]
	provider, ok := c.unwrap[kekID]
	if !ok {
		return "", fmt.Errorf("secret requires unavailable key provider %q", kekID)
	}
	wrapped, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return "", err
	}
	sealed, err := base64.StdEncoding.DecodeString(parts[3])
	if err != nil {
		return "", err
	}

	dek, err := provider.UnwrapDEK(ctx, wrapped)
	if err != nil {
		return "", err
	}
	defer func() {
		for i := range dek {
			dek[i] = 0
		}
	}()
	block, err := aes.NewCipher(dek)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(sealed) < gcm.NonceSize() {
		return "", errors.New("encrypted secret payload is invalid")
	}
	plain, err := gcm.Open(nil, sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func (c *SecretCipher) openLegacy(encoded string) (string, error) {
	payload, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	if len(payload) < c.legacy.NonceSize() {
		return "", errors.New("encrypted secret payload is invalid")
	}
	plain, err := c.legacy.Open(nil, payload[:c.legacy.NonceSize()], payload[c.legacy.NonceSize():], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
