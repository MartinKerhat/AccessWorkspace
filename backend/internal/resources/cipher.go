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

	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/nacl/box"
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
	// SecretClassPersonal wraps the DEK to the owner's vault PUBLIC key
	// (X25519 sealed box) — anyone can write, only the owner's unlocked
	// private key reads. kek-id is `owner`; never resolvable server-side.
	SecretClassPersonal SecretClass = "personal"
)

const personalKekID = "owner"

// ErrVaultLocked is returned when a personal secret is read without the
// owner's vault private key in the session.
var ErrVaultLocked = errors.New("personal vault is locked")

// IsPersonalEnvelope reports whether the stored value is a personal-class v2
// envelope (readable only with the owner's vault key). Startup migrations
// must skip these — the server alone can never decrypt them, by design.
func IsPersonalEnvelope(value string) bool {
	return strings.HasPrefix(strings.TrimSpace(value), envelopePrefix+string(SecretClassPersonal)+":")
}

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

// UseKEKProvider makes the given provider wrap all NEW DEKs while keeping
// every previously registered provider available for reads — existing rows
// stay decryptable and are migrated by rewrap, not by re-encryption. Call
// during startup wiring only (not concurrency-safe afterwards).
func (c *SecretCipher) UseKEKProvider(provider KEKProvider) {
	c.kek = provider
	c.unwrap[provider.ID()] = provider
}

// RegisterKEKProvider makes a provider available for UNWRAPPING without
// making it wrap new DEKs. Used when rolling back to another provider: rows
// wrapped by this one stay readable and get rewrapped by the startup
// migration. Call during startup wiring only.
func (c *SecretCipher) RegisterKEKProvider(provider KEKProvider) {
	c.unwrap[provider.ID()] = provider
}

// NeedsRewrap reports whether a stored v2 value's DEK is wrapped by a
// provider other than the active one (e.g. `local` rows after the deployment
// switched to the Key Vault KEK).
func (c *SecretCipher) NeedsRewrap(value string) bool {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, envelopePrefix) {
		return false
	}
	parts := strings.SplitN(strings.TrimPrefix(trimmed, envelopePrefix), ":", 4)
	return len(parts) == 4 && parts[1] != c.kek.ID()
}

// RewrapForStorage re-wraps a v2 envelope's DEK under the active provider
// without touching the sealed value — the plaintext is never reconstructed.
// Values that are not v2 or already use the active provider pass through.
func (c *SecretCipher) RewrapForStorage(ctx context.Context, value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if !c.NeedsRewrap(trimmed) {
		return trimmed, nil
	}
	parts := strings.SplitN(strings.TrimPrefix(trimmed, envelopePrefix), ":", 4)
	class, oldKekID := parts[0], parts[1]
	provider, ok := c.unwrap[oldKekID]
	if !ok {
		return "", fmt.Errorf("secret requires unavailable key provider %q", oldKekID)
	}
	wrapped, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return "", err
	}
	dek, err := provider.UnwrapDEK(ctx, wrapped)
	if err != nil {
		return "", err
	}
	rewrapped, err := c.kek.WrapDEK(ctx, dek)
	for i := range dek {
		dek[i] = 0
	}
	if err != nil {
		return "", err
	}
	return envelopePrefix + class + ":" + c.kek.ID() + ":" +
		base64.StdEncoding.EncodeToString(rewrapped) + ":" + parts[3], nil
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

// EncryptPersonalForStorage seals the value into a personal-class envelope:
// fresh DEK for the value, DEK sealed to the owner's vault public key. Works
// without any unlock — writing into a vault never requires opening it.
// Already-encrypted values pass through (same idempotence as shared).
func (c *SecretCipher) EncryptPersonalForStorage(ctx context.Context, value string, ownerPublicKey []byte) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	if IsEncryptedForStorage(trimmed) {
		return trimmed, nil
	}
	if len(ownerPublicKey) != 32 {
		return "", errors.New("owner vault public key is invalid")
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

	var pub [32]byte
	copy(pub[:], ownerPublicKey)
	wrapped, err := box.SealAnonymous(nil, dek, &pub, rand.Reader)
	for i := range dek {
		dek[i] = 0
	}
	if err != nil {
		return "", err
	}

	return envelopePrefix + string(SecretClassPersonal) + ":" + personalKekID + ":" +
		base64.StdEncoding.EncodeToString(wrapped) + ":" +
		base64.StdEncoding.EncodeToString(sealed), nil
}

// DecryptPersonalFromStorage opens a personal-class envelope with the
// owner's vault private key (nil/empty key → ErrVaultLocked). Non-personal
// values fall through to the regular decrypt path so callers can use this
// as the single read entrypoint for resource secrets.
func (c *SecretCipher) DecryptPersonalFromStorage(ctx context.Context, value string, ownerPrivateKey []byte) (string, error) {
	trimmed := strings.TrimSpace(value)
	if !IsPersonalEnvelope(trimmed) {
		return c.DecryptFromStorage(ctx, trimmed)
	}
	if len(ownerPrivateKey) != 32 {
		return "", ErrVaultLocked
	}
	parts := strings.SplitN(strings.TrimPrefix(trimmed, envelopePrefix), ":", 4)
	if len(parts) != 4 {
		return "", errors.New("encrypted secret envelope is invalid")
	}
	wrapped, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return "", err
	}
	sealed, err := base64.StdEncoding.DecodeString(parts[3])
	if err != nil {
		return "", err
	}

	var priv, pub [32]byte
	copy(priv[:], ownerPrivateKey)
	curve25519.ScalarBaseMult(&pub, &priv)
	dek, ok := box.OpenAnonymous(nil, wrapped, &pub, &priv)
	if !ok {
		return "", errors.New("personal secret does not belong to this vault")
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
	if parts[0] == string(SecretClassPersonal) {
		// Personal envelopes never open through the server-side key chain.
		return "", ErrVaultLocked
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
