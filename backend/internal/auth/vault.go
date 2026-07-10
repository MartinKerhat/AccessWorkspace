package auth

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

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/nacl/box"
)

// Personal vault key chain. Every user gets an X25519 keypair:
//
//   - public_key: stored plaintext — encrypting a personal secret needs no
//     unlock (the extension "save as personal" flow is always silent).
//   - private key: never stored bare. It exists wrapped by unlock methods
//     (the local login password today; passphrase/passkey later) and, while
//     a session is "unlocked", wrapped under the raw session token. The DB
//     stores only the token's hash, so a dump cannot open any of the wraps —
//     only a live authenticated request or the owner's unlock secret can.
//
// Local-account users never see any of this: their vault is created and
// unlocked automatically at login from the password they just typed.

const vaultUnlockMethodPassword = "password"

// argon2id parameters for password-derived wrap keys (interactive profile).
const (
	vaultArgonTime    = 1
	vaultArgonMemory  = 64 * 1024
	vaultArgonThreads = 4
)

// deriveSessionWrapKey turns a raw bearer token into the AES key that wraps
// the vault private key inside that session's row. Domain-separated from the
// token-hash used for lookups.
func deriveSessionWrapKey(token string) []byte {
	sum := sha256.Sum256([]byte("vault-session:" + strings.TrimSpace(token)))
	return sum[:]
}

func derivePasswordWrapKey(password string, salt []byte) []byte {
	return argon2.IDKey([]byte(password), salt, vaultArgonTime, vaultArgonMemory, vaultArgonThreads, 32)
}

func vaultSeal(key, plaintext []byte) (string, error) {
	block, err := aes.NewCipher(key)
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
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, plaintext, nil)), nil
}

func vaultOpen(key []byte, encoded string) ([]byte, error) {
	payload, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(payload) < gcm.NonceSize() {
		return nil, errors.New("vault payload is invalid")
	}
	return gcm.Open(nil, payload[:gcm.NonceSize()], payload[gcm.NonceSize():], nil)
}

// VaultPublicKey returns the user's vault public key, or nil when the user
// has no vault yet. Satisfies the resources package's key lookup interface.
func (r *Repository) VaultPublicKey(ctx context.Context, userID string) ([]byte, error) {
	var encoded string
	err := r.db.QueryRow(ctx, `
		select public_key from user_vaults where user_id = $1
	`, strings.TrimSpace(userID)).Scan(&encoded)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return base64.StdEncoding.DecodeString(encoded)
}

// EnsureLocalUserVault creates the user's vault on first login (or first
// account creation) using the login password as the unlock method. Existing
// vaults are left untouched. Returns the private key when it is available
// in this call (newly created, or unlocked with the given password), nil
// otherwise.
func (r *Repository) EnsureLocalUserVault(ctx context.Context, userID, password string) ([]byte, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" || password == "" {
		return nil, nil
	}

	var publicKey string
	err := r.db.QueryRow(ctx, `select public_key from user_vaults where user_id = $1`, userID).Scan(&publicKey)
	switch {
	case err == nil:
		// Vault exists: try unlocking with the password method.
		return r.unlockVaultWithPassword(ctx, userID, password)
	case errors.Is(err, pgx.ErrNoRows):
		// Create the vault.
	default:
		return nil, err
	}

	pub, priv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}
	wrapped, err := vaultSeal(derivePasswordWrapKey(password, salt), priv[:])
	if err != nil {
		return nil, err
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	if _, err := tx.Exec(ctx, `
		insert into user_vaults (user_id, public_key)
		values ($1, $2)
		on conflict (user_id) do nothing
	`, userID, base64.StdEncoding.EncodeToString(pub[:])); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		insert into user_vault_unlocks (user_id, method, label, wrapped_private_key, salt)
		values ($1, $2, '', $3, $4)
		on conflict (user_id, method, label) do nothing
	`, userID, vaultUnlockMethodPassword, wrapped, base64.StdEncoding.EncodeToString(salt)); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	out := append([]byte(nil), priv[:]...)
	return out, nil
}

// unlockVaultWithPassword opens the password-wrapped private key. A missing
// password unlock method or a non-matching wrap yields (nil, nil) — the
// vault simply stays locked for this session; it is not a login failure.
func (r *Repository) unlockVaultWithPassword(ctx context.Context, userID, password string) ([]byte, error) {
	var wrapped, salt string
	err := r.db.QueryRow(ctx, `
		select wrapped_private_key, salt from user_vault_unlocks
		where user_id = $1 and method = $2 and label = ''
	`, userID, vaultUnlockMethodPassword).Scan(&wrapped, &salt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	saltBytes, err := base64.StdEncoding.DecodeString(salt)
	if err != nil {
		return nil, fmt.Errorf("vault unlock salt is invalid: %w", err)
	}
	priv, err := vaultOpen(derivePasswordWrapKey(password, saltBytes), wrapped)
	if err != nil {
		// Wrap predates a password change we did not process — locked, not fatal.
		return nil, nil
	}
	return priv, nil
}

// attachVaultKeyToSession stores the private key on a session row, wrapped
// under the raw session token. table must be one of the session tables.
func (r *Repository) attachVaultKeyToSession(ctx context.Context, table, token string, privateKey []byte) error {
	if len(privateKey) == 0 {
		return nil
	}
	if table != "auth_sessions" && table != "browser_extension_sessions" {
		return fmt.Errorf("invalid session table %q", table)
	}
	wrapped, err := vaultSeal(deriveSessionWrapKey(token), privateKey)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		update `+table+` set vault_private_key = $2 where token = $1
	`, hashToken(token), wrapped)
	return err
}

// attachVaultKeyToCurrentSession writes the unlocked private key onto
// whichever session row (web or extension) the raw token belongs to.
func (r *Repository) attachVaultKeyToCurrentSession(ctx context.Context, token string, privateKey []byte) error {
	if len(privateKey) == 0 {
		return nil
	}
	wrapped, err := vaultSeal(deriveSessionWrapKey(token), privateKey)
	if err != nil {
		return err
	}
	hash := hashToken(token)
	if _, err := r.db.Exec(ctx, `update auth_sessions set vault_private_key = $2 where token = $1`, hash, wrapped); err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `update browser_extension_sessions set vault_private_key = $2 where token = $1`, hash, wrapped)
	return err
}

// clearSessionVaultKey re-locks the vault for the current session by dropping
// its stored key. Other sessions keep their own key.
func (r *Repository) clearSessionVaultKey(ctx context.Context, token string) error {
	hash := hashToken(token)
	if _, err := r.db.Exec(ctx, `update auth_sessions set vault_private_key = '' where token = $1`, hash); err != nil {
		return err
	}
	_, err := r.db.Exec(ctx, `update browser_extension_sessions set vault_private_key = '' where token = $1`, hash)
	return err
}

// openSessionVaultKey unwraps a session-carried private key. Returns nil on
// any failure — the vault is simply locked for this request.
func openSessionVaultKey(token, wrapped string) []byte {
	if strings.TrimSpace(wrapped) == "" {
		return nil
	}
	priv, err := vaultOpen(deriveSessionWrapKey(token), wrapped)
	if err != nil {
		return nil
	}
	return priv
}
