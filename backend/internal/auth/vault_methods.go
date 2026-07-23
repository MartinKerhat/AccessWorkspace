package auth

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Vault unlock-method management: list / remove / rename the wraps of the
// vault private key (see vault.go for the key chain). Add lives with each
// method's own file (AddPassphraseMethod, AddPasskeyMethod); this file holds
// the method-agnostic operations behind the vault settings UI.
//
// Every operation here requires the caller to hold the unlocked private key
// (enforced at the Service layer) — the session token alone must not be able
// to reshape or shrink the set of doors to the vault.

const vaultNicknameMaxLength = 64

// sanitizeVaultNickname trims and caps a user-supplied method name. Empty is
// fine (the UI falls back to method + created date).
func sanitizeVaultNickname(nickname string) string {
	nickname = strings.TrimSpace(nickname)
	if len(nickname) > vaultNicknameMaxLength {
		nickname = nickname[:vaultNicknameMaxLength]
	}
	return nickname
}

// VaultMethodDetail describes one unlock method for the management UI.
// Nothing here is secret: label is the passkey credential ID (public),
// nickname is user-chosen display text.
type VaultMethodDetail struct {
	Method    string    `json:"method"`
	Label     string    `json:"label"`
	Nickname  string    `json:"nickname"`
	CreatedAt time.Time `json:"createdAt"`
}

// ListUnlockMethods returns every unlock method on the user's vault.
func (r *Repository) ListUnlockMethods(ctx context.Context, userID string) ([]VaultMethodDetail, error) {
	rows, err := r.db.Query(ctx, `
		select method, label, nickname, created_at from user_vault_unlocks
		where user_id = $1
		order by method, created_at
	`, strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []VaultMethodDetail{}
	for rows.Next() {
		var detail VaultMethodDetail
		if err := rows.Scan(&detail.Method, &detail.Label, &detail.Nickname, &detail.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, detail)
	}
	return out, rows.Err()
}

// RemoveUnlockMethod deletes one wrap of the vault private key. Refused for
// the system-managed login-password wrap (it would desync the vault from the
// login) and when it would leave the vault with no unlock method at all —
// the private key is random and underivable, so a vault behind zero doors is
// lost, not locked.
func (r *Repository) RemoveUnlockMethod(ctx context.Context, userID, method, label string) error {
	userID = strings.TrimSpace(userID)
	method = strings.TrimSpace(method)
	label = strings.TrimSpace(label)
	if userID == "" || method == "" {
		return ErrInvalidInput
	}
	if method == vaultUnlockMethodPassword {
		return fmt.Errorf("%w: the login password unlock cannot be removed", ErrInvalidInput)
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	// Count inside the transaction so two concurrent removals cannot race
	// past the last-door guard.
	var total int
	if err := tx.QueryRow(ctx, `
		select count(*) from user_vault_unlocks where user_id = $1
	`, userID).Scan(&total); err != nil {
		return err
	}
	if total <= 1 {
		return fmt.Errorf("%w: the last unlock method cannot be removed", ErrInvalidInput)
	}

	tag, err := tx.Exec(ctx, `
		delete from user_vault_unlocks
		where user_id = $1 and method = $2 and label = $3
	`, userID, method, label)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: unlock method not found", ErrInvalidInput)
	}
	return tx.Commit(ctx)
}

// RenamePasskeyMethod updates the display name of one enrolled passkey.
// Passkeys are the only method a user holds several of (one per device), so
// they are the only rows that take a name; passphrase/password rows have
// fixed display names in the UI.
func (r *Repository) RenamePasskeyMethod(ctx context.Context, userID, credentialID, nickname string) error {
	userID = strings.TrimSpace(userID)
	credentialID = strings.TrimSpace(credentialID)
	if userID == "" || credentialID == "" {
		return ErrInvalidInput
	}
	tag, err := r.db.Exec(ctx, `
		update user_vault_unlocks
		set nickname = $4, updated_at = now()
		where user_id = $1 and method = $2 and label = $3
	`, userID, vaultUnlockMethodPasskey, credentialID, sanitizeVaultNickname(nickname))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: passkey not found", ErrInvalidInput)
	}
	return nil
}
