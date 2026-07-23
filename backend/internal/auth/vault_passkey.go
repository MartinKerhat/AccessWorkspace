package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/nacl/box"
)

// Vault passkey (WebAuthn PRF): the "nothing to remember" unlock. A platform
// authenticator (Windows Hello, Touch ID, a security key) holds a credential
// whose PRF extension yields a stable 32-byte secret after a local user
// check (face / fingerprint / PIN). That secret never leaves the device
// except as the PRF output, which the browser sends to us over TLS to wrap
// (setup) or unwrap (unlock) the vault private key — same server-side trust
// model as the passphrase, but with no memorized secret.
//
// The passkey is used purely as a key-storage device, NOT as an auth factor
// to our server: the session token already authenticates the user, and the
// crypto gate is the unwrap itself (only the real PRF output opens the key),
// so we do not verify the WebAuthn assertion server-side. That keeps the
// server free of attestation/sign-count bookkeeping. label holds the
// credential ID (base64url); salt holds the non-secret PRF input salt.
const vaultUnlockMethodPasskey = "passkey"

// derivePasskeyWrapKey turns the high-entropy 32-byte PRF output into the AES
// wrap key. A fast hash is appropriate here (unlike passphrases, the input is
// already uniform) and domain-separates it from other uses.
func derivePasskeyWrapKey(prfSecret []byte) []byte {
	sum := sha256.Sum256(append([]byte("vault-passkey:"), prfSecret...))
	return sum[:]
}

// PasskeyDescriptor tells the browser how to run the unlock ceremony for one
// registered passkey: which credential to request and which PRF salt to eval.
type PasskeyDescriptor struct {
	CredentialID string `json:"credentialId"`
	PRFSalt      string `json:"prfSalt"`
}

func decodePRFSecret(prfSecretB64 string) ([]byte, error) {
	secret, err := base64.StdEncoding.DecodeString(strings.TrimSpace(prfSecretB64))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid passkey secret", ErrInvalidInput)
	}
	if len(secret) < 16 {
		return nil, fmt.Errorf("%w: passkey secret too short", ErrInvalidInput)
	}
	return secret, nil
}

// ListPasskeyDescriptors returns the credential/salt pairs the browser needs
// to build a WebAuthn get() for unlocking.
func (r *Repository) ListPasskeyDescriptors(ctx context.Context, userID string) ([]PasskeyDescriptor, error) {
	rows, err := r.db.Query(ctx, `
		select label, salt from user_vault_unlocks
		where user_id = $1 and method = $2
	`, strings.TrimSpace(userID), vaultUnlockMethodPasskey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PasskeyDescriptor{}
	for rows.Next() {
		var label, salt string
		if err := rows.Scan(&label, &salt); err != nil {
			return nil, err
		}
		out = append(out, PasskeyDescriptor{CredentialID: label, PRFSalt: salt})
	}
	return out, rows.Err()
}

// SetupVaultWithPasskey creates a vault for a user who has none, wrapping the
// private key under the passkey's PRF secret — no passphrase involved.
func (r *Repository) SetupVaultWithPasskey(ctx context.Context, userID, credentialID, prfSalt, prfSecretB64, nickname string) ([]byte, error) {
	userID = strings.TrimSpace(userID)
	credentialID = strings.TrimSpace(credentialID)
	if userID == "" || credentialID == "" {
		return nil, ErrInvalidInput
	}
	prfSecret, err := decodePRFSecret(prfSecretB64)
	if err != nil {
		return nil, err
	}
	var exists bool
	if err := r.db.QueryRow(ctx, `select exists(select 1 from user_vaults where user_id = $1)`, userID).Scan(&exists); err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("%w: vault already exists", ErrInvalidInput)
	}

	pub, priv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	wrapped, err := vaultSeal(derivePasskeyWrapKey(prfSecret), priv[:])
	if err != nil {
		return nil, err
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		insert into user_vaults (user_id, public_key) values ($1, $2)
		on conflict (user_id) do nothing
	`, userID, base64.StdEncoding.EncodeToString(pub[:])); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		insert into user_vault_unlocks (user_id, method, label, wrapped_private_key, salt, nickname)
		values ($1, $2, $3, $4, $5, $6)
		on conflict (user_id, method, label) do update
		set wrapped_private_key = excluded.wrapped_private_key, salt = excluded.salt,
		    nickname = excluded.nickname, updated_at = now()
	`, userID, vaultUnlockMethodPasskey, credentialID, wrapped, prfSalt, sanitizeVaultNickname(nickname)); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return append([]byte(nil), priv[:]...), nil
}

// AddPasskeyMethod registers an additional passkey against an already-unlocked
// vault (a second device, or adding Hello to a passphrase-only vault).
func (r *Repository) AddPasskeyMethod(ctx context.Context, userID string, privateKey []byte, credentialID, prfSalt, prfSecretB64, nickname string) error {
	if len(privateKey) != 32 {
		return fmt.Errorf("%w: vault is locked", ErrInvalidInput)
	}
	credentialID = strings.TrimSpace(credentialID)
	if credentialID == "" {
		return ErrInvalidInput
	}
	prfSecret, err := decodePRFSecret(prfSecretB64)
	if err != nil {
		return err
	}
	wrapped, err := vaultSeal(derivePasskeyWrapKey(prfSecret), privateKey)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		insert into user_vault_unlocks (user_id, method, label, wrapped_private_key, salt, nickname)
		values ($1, $2, $3, $4, $5, $6)
		on conflict (user_id, method, label) do update
		set wrapped_private_key = excluded.wrapped_private_key, salt = excluded.salt,
		    nickname = excluded.nickname, updated_at = now()
	`, strings.TrimSpace(userID), vaultUnlockMethodPasskey, credentialID, wrapped, prfSalt, sanitizeVaultNickname(nickname))
	return err
}

// UnlockVaultWithPasskey opens the vault using a credential's PRF output.
// Returns nil (not an error) when the secret does not open the row — the
// caller reports a failed unlock.
func (r *Repository) UnlockVaultWithPasskey(ctx context.Context, userID, credentialID, prfSecretB64 string) ([]byte, error) {
	prfSecret, err := decodePRFSecret(prfSecretB64)
	if err != nil {
		return nil, err
	}
	var wrapped string
	err = r.db.QueryRow(ctx, `
		select wrapped_private_key from user_vault_unlocks
		where user_id = $1 and method = $2 and label = $3
	`, strings.TrimSpace(userID), vaultUnlockMethodPasskey, strings.TrimSpace(credentialID)).Scan(&wrapped)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	priv, err := vaultOpen(derivePasskeyWrapKey(prfSecret), wrapped)
	if err != nil {
		return nil, nil
	}
	return priv, nil
}
