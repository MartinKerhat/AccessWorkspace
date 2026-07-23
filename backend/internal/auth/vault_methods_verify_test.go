package auth

// End-to-end verification of vault unlock-method management guards against a
// throwaway database. Runs only when VERIFY_DATABASE_URL is set (same pattern
// as lockout_verify_test.go).

import (
	"context"
	"errors"
	"os"
	"testing"

	"access-workspace/backend/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestVaultMethodManagementEndToEnd(t *testing.T) {
	dsn := os.Getenv("VERIFY_DATABASE_URL")
	if dsn == "" {
		t.Skip("VERIFY_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()
	if err := db.RunMigrations(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	repo := NewRepository(pool)
	const userID = "vault-methods-user"
	if _, err := pool.Exec(ctx, `
		insert into app_users (id, username, display_name, email, password_hash)
		values ($1, $1, 'Vault Methods User', 'vault-methods@example.com', 'x')
		on conflict (id) do nothing
	`, userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	// Fresh vault each run. Unlock rows key on app_users, not user_vaults,
	// so both tables need clearing (mirrors production vault destruction).
	if _, err := pool.Exec(ctx, `delete from user_vault_unlocks where user_id = $1`, userID); err != nil {
		t.Fatalf("reset unlocks: %v", err)
	}
	if _, err := pool.Exec(ctx, `delete from user_vaults where user_id = $1`, userID); err != nil {
		t.Fatalf("reset vault: %v", err)
	}

	// Passphrase-only vault.
	priv, err := repo.SetupVaultWithPassphrase(ctx, userID, "test-passphrase-1")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Last-door guard: the only method cannot be removed.
	if err := repo.RemoveUnlockMethod(ctx, userID, "passphrase", ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected last-door refusal, got %v", err)
	}

	// Add two passkeys with nicknames (fake PRF secrets are fine — wrap only).
	prfSecret := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=" // 32 zero bytes
	if err := repo.AddPasskeyMethod(ctx, userID, priv, "cred-laptop", "salt1", prfSecret, "Work laptop"); err != nil {
		t.Fatalf("add passkey 1: %v", err)
	}
	if err := repo.AddPasskeyMethod(ctx, userID, priv, "cred-phone", "salt2", prfSecret, "Phone"); err != nil {
		t.Fatalf("add passkey 2: %v", err)
	}

	details, err := repo.ListUnlockMethods(ctx, userID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(details) != 3 {
		t.Fatalf("expected 3 methods, got %d: %+v", len(details), details)
	}
	byLabel := map[string]VaultMethodDetail{}
	for _, d := range details {
		byLabel[d.Label] = d
	}
	if byLabel["cred-laptop"].Nickname != "Work laptop" {
		t.Fatalf("nickname not stored: %+v", byLabel["cred-laptop"])
	}

	// Rename an enrolled passkey.
	if err := repo.RenamePasskeyMethod(ctx, userID, "cred-laptop", "Old laptop"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	// Renaming an unknown credential errors.
	if err := repo.RenamePasskeyMethod(ctx, userID, "cred-missing", "x"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected rename-missing refusal, got %v", err)
	}

	// Remove one passkey; the other + passphrase remain.
	if err := repo.RemoveUnlockMethod(ctx, userID, "passkey", "cred-laptop"); err != nil {
		t.Fatalf("remove passkey: %v", err)
	}
	// Removing it again reports not-found.
	if err := repo.RemoveUnlockMethod(ctx, userID, "passkey", "cred-laptop"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected not-found refusal, got %v", err)
	}
	// Removed wrap no longer unlocks.
	if got, err := repo.UnlockVaultWithPasskey(ctx, userID, "cred-laptop", prfSecret); err != nil || got != nil {
		t.Fatalf("removed passkey still unlocks: key=%v err=%v", got, err)
	}
	// Remaining passkey still opens the same private key.
	got, err := repo.UnlockVaultWithPasskey(ctx, userID, "cred-phone", prfSecret)
	if err != nil || string(got) != string(priv) {
		t.Fatalf("surviving passkey should unlock the same key (err=%v)", err)
	}

	// The passphrase is removable while the passkey remains…
	if err := repo.RemoveUnlockMethod(ctx, userID, "passphrase", ""); err != nil {
		t.Fatalf("remove passphrase: %v", err)
	}
	// …but the now-last passkey is not.
	if err := repo.RemoveUnlockMethod(ctx, userID, "passkey", "cred-phone"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected last-door refusal on final passkey, got %v", err)
	}
}

// TestRemoveUnlockMethodRefusesPassword: the login-password guard (and the
// empty-input guard) fire before any database access, so this always runs.
func TestRemoveUnlockMethodRefusesPassword(t *testing.T) {
	repo := NewRepository(nil)
	if err := repo.RemoveUnlockMethod(context.Background(), "anyone", "password", ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected password-method refusal, got %v", err)
	}
	if err := repo.RemoveUnlockMethod(context.Background(), "", "passphrase", ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected empty-user refusal, got %v", err)
	}
}
