package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/nacl/box"
)

// Vault passphrase: the SSO-user unlock method. SSO logins carry no password
// the server can derive a key from, so those users set a vault passphrase on
// first personal-secret use and enter it once per session. It is a peer of
// the local-login "password" method — same Argon2id wrap, different label —
// so a vault can carry both. (Windows Hello / passkey PRF, when it ships,
// becomes a third method: another wrap of the same private key.)
const vaultUnlockMethodPassphrase = "passphrase"

type VaultStatus struct {
	HasVault bool     `json:"hasVault"`
	Unlocked bool     `json:"unlocked"`
	Methods  []string `json:"methods"`
}

// VaultStatus reports whether the user has a vault and which unlock methods
// exist. Unlocked is filled by the caller from the session.
func (r *Repository) VaultStatus(ctx context.Context, userID string) (VaultStatus, error) {
	userID = strings.TrimSpace(userID)
	var exists bool
	if err := r.db.QueryRow(ctx, `select exists(select 1 from user_vaults where user_id = $1)`, userID).Scan(&exists); err != nil {
		return VaultStatus{}, err
	}
	status := VaultStatus{HasVault: exists, Methods: []string{}}
	if !exists {
		return status, nil
	}
	rows, err := r.db.Query(ctx, `select distinct method from user_vault_unlocks where user_id = $1 order by method`, userID)
	if err != nil {
		return VaultStatus{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var method string
		if err := rows.Scan(&method); err != nil {
			return VaultStatus{}, err
		}
		status.Methods = append(status.Methods, method)
	}
	return status, rows.Err()
}

// SetupVaultWithPassphrase creates a vault for a user who has none, wrapping
// the private key under a passphrase. Errors if a vault already exists (use
// AddPassphraseMethod from an unlocked session to add a passphrase later).
func (r *Repository) SetupVaultWithPassphrase(ctx context.Context, userID, passphrase string) ([]byte, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, ErrInvalidInput
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
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}
	wrapped, err := vaultSeal(derivePasswordWrapKey(passphrase, salt), priv[:])
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
		insert into user_vaults (user_id, public_key) values ($1, $2)
		on conflict (user_id) do nothing
	`, userID, base64.StdEncoding.EncodeToString(pub[:])); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		insert into user_vault_unlocks (user_id, method, label, wrapped_private_key, salt)
		values ($1, $2, '', $3, $4)
		on conflict (user_id, method, label) do nothing
	`, userID, vaultUnlockMethodPassphrase, wrapped, base64.StdEncoding.EncodeToString(salt)); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return append([]byte(nil), priv[:]...), nil
}

// AddPassphraseMethod wraps an already-unlocked private key under a
// passphrase, so a local user can also unlock via passphrase (e.g. in the
// extension) without their login password.
func (r *Repository) AddPassphraseMethod(ctx context.Context, userID string, privateKey []byte, passphrase string) error {
	if len(privateKey) != 32 {
		return fmt.Errorf("%w: vault is locked", ErrInvalidInput)
	}
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return err
	}
	wrapped, err := vaultSeal(derivePasswordWrapKey(passphrase, salt), privateKey)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		insert into user_vault_unlocks (user_id, method, label, wrapped_private_key, salt)
		values ($1, $2, '', $3, $4)
		on conflict (user_id, method, label) do update
		set wrapped_private_key = excluded.wrapped_private_key, salt = excluded.salt, updated_at = now()
	`, strings.TrimSpace(userID), vaultUnlockMethodPassphrase, wrapped, base64.StdEncoding.EncodeToString(salt))
	return err
}

// UnlockVaultWithSecret tries the supplied secret against every Argon2id
// unlock method the user has (password + passphrase). Returns the private
// key on the first match, or nil if none opens (wrong secret / no vault) —
// the caller reports that as a failed unlock, not a fatal error.
func (r *Repository) UnlockVaultWithSecret(ctx context.Context, userID, secret string) ([]byte, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" || secret == "" {
		return nil, nil
	}
	rows, err := r.db.Query(ctx, `
		select wrapped_private_key, salt from user_vault_unlocks
		where user_id = $1 and method in ($2, $3)
	`, userID, vaultUnlockMethodPassword, vaultUnlockMethodPassphrase)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()

	type wrap struct{ wrapped, salt string }
	var wraps []wrap
	for rows.Next() {
		var w wrap
		if err := rows.Scan(&w.wrapped, &w.salt); err != nil {
			return nil, err
		}
		wraps = append(wraps, w)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, w := range wraps {
		saltBytes, err := base64.StdEncoding.DecodeString(w.salt)
		if err != nil {
			continue
		}
		if priv, err := vaultOpen(derivePasswordWrapKey(secret, saltBytes), w.wrapped); err == nil {
			return priv, nil
		}
	}
	return nil, nil
}
