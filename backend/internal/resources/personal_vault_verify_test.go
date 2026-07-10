package resources

// End-to-end verification of the personal vault key chain (phase 4 slice 1):
// local login creates + unlocks the vault, personal secrets are sealed to the
// owner's public key, sessions carry the private key, the extension connect
// flow hands the unlock over, and nothing server-side can decrypt without it.
// Runs only when VERIFY_DATABASE_URL points at a throwaway database.

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"access-workspace/backend/internal/auth"
	"access-workspace/backend/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPersonalVaultEndToEnd(t *testing.T) {
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

	authRepo := auth.NewRepository(pool)
	authService := auth.NewService(authRepo, auth.ModeDev, "")
	secretCipher := NewSecretCipher("verify-test-key-000000000000000000000000")
	resourceRepo := NewRepository(pool)
	service := NewService(resourceRepo, &captureAuditLogger{}, fakeKeyVaultResolver{}, nil, nil, secretCipher, authRepo)

	const password = "vault-test-password-1"
	created, err := authService.CreateUser(ctx, auth.CreateUserInput{
		Username:    "vault-owner",
		DisplayName: "Vault Owner",
		Email:       "vault-owner@example.com",
		Password:    password,
		IsAdmin:     true,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Vault exists from account creation (public key on file).
	publicKey, err := authRepo.VaultPublicKey(ctx, created.ID)
	if err != nil || len(publicKey) != 32 {
		t.Fatalf("expected 32-byte vault public key after user creation, got %d bytes err=%v", len(publicKey), err)
	}

	// Local login unlocks the vault for the session.
	login, err := authService.Login(ctx, "vault-owner", password)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if len(login.User.VaultPrivateKey) != 32 {
		t.Fatalf("expected unlocked vault on login result")
	}
	sessionUser, err := authRepo.UserByToken(ctx, login.Token)
	if err != nil || len(sessionUser.VaultPrivateKey) != 32 {
		t.Fatalf("expected session to carry the vault key, err=%v", err)
	}

	// Personal secret is sealed to the owner's public key at rest.
	resource, err := service.Create(ctx, login.User, CreateResourceInput{
		Name:          "my-portal-login",
		Type:          TypeSharedSecret,
		Personal:      true,
		Owner:         "Vault Owner",
		Username:      "owner@example.com",
		SecretMode:    SecretModeInline,
		SecretValue:   "personal-pw-123",
		RevealAllowed: true,
	})
	if err != nil {
		t.Fatalf("create personal resource: %v", err)
	}
	var stored string
	if err := pool.QueryRow(ctx, `select secret_value from resource_secrets where resource_id = $1`, resource.ID).Scan(&stored); err != nil {
		t.Fatalf("read stored: %v", err)
	}
	if !strings.HasPrefix(stored, "enc:v2:personal:owner:") {
		t.Fatalf("expected personal envelope at rest, got %q", stored)
	}

	// Owner with unlocked session reveals; server-side chain alone cannot.
	reveal, err := service.Reveal(ctx, sessionUser, resource.ID)
	if err != nil || reveal.SecretValue != "personal-pw-123" {
		t.Fatalf("owner reveal: got %q err=%v", reveal.SecretValue, err)
	}
	if _, err := secretCipher.DecryptFromStorage(ctx, stored); !errors.Is(err, ErrVaultLocked) {
		t.Fatalf("expected server-side decrypt to be vault-locked, got %v", err)
	}
	lockedUser := sessionUser
	lockedUser.VaultPrivateKey = nil
	if _, err := service.Reveal(ctx, lockedUser, resource.ID); !errors.Is(err, ErrVaultLocked) {
		t.Fatalf("expected locked session reveal to fail with ErrVaultLocked, got %v", err)
	}

	// A second login (fresh session) unlocks via the password method.
	secondLogin, err := authService.Login(ctx, "vault-owner", password)
	if err != nil {
		t.Fatalf("second login: %v", err)
	}
	secondUser, err := authRepo.UserByToken(ctx, secondLogin.Token)
	if err != nil || len(secondUser.VaultPrivateKey) != 32 {
		t.Fatalf("expected second session unlocked, err=%v", err)
	}
	if reveal, err := service.Reveal(ctx, secondUser, resource.ID); err != nil || reveal.SecretValue != "personal-pw-123" {
		t.Fatalf("second session reveal: got %q err=%v", reveal.SecretValue, err)
	}

	// Extension connect hands the unlock over to the extension session.
	connect, err := authService.IssueBrowserExtensionConnectToken(ctx, sessionUser, auth.ModeDev)
	if err != nil {
		t.Fatalf("issue connect token: %v", err)
	}
	extLogin, err := authService.ExchangeBrowserExtensionConnectToken(ctx, connect.Token, "install-e2e")
	if err != nil {
		t.Fatalf("exchange connect token: %v", err)
	}
	extUser, err := authRepo.UserByToken(ctx, extLogin.Token)
	if err != nil || len(extUser.VaultPrivateKey) != 32 {
		t.Fatalf("expected extension session to carry the vault key, err=%v", err)
	}
	if reveal, err := service.Reveal(ctx, extUser, resource.ID); err != nil || reveal.SecretValue != "personal-pw-123" {
		t.Fatalf("extension session reveal: got %q err=%v", reveal.SecretValue, err)
	}

	// Startup migration must never touch personal envelopes.
	if err := resourceRepo.UpgradeSecretEncryption(ctx, secretCipher, authRepo); err != nil {
		t.Fatalf("migration with personal rows present: %v", err)
	}
	var after string
	if err := pool.QueryRow(ctx, `select secret_value from resource_secrets where resource_id = $1`, resource.ID).Scan(&after); err != nil {
		t.Fatalf("reread stored: %v", err)
	}
	if after != stored {
		t.Fatalf("migration modified a personal envelope")
	}

	// Slice 4: a personal secret from BEFORE the vault existed (stored
	// shared-class, org-readable) is converted into the owner's vault by the
	// startup migration — server-side, using only the public key.
	legacyShared, err := secretCipher.EncryptForStorage(ctx, "pre-vault-personal-pw", SecretClassShared)
	if err != nil {
		t.Fatalf("encrypt legacy shared: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		insert into resources (id, name, type, personal, owner_user_id, reveal_allowed)
		values ('legacy-personal', 'legacy-personal', 'shared_secret', true, $1, true)
		on conflict (id) do nothing
	`, created.ID); err != nil {
		t.Fatalf("seed legacy personal resource: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		insert into resource_secrets (resource_id, secret_mode, secret_value)
		values ('legacy-personal', 'inline', $1)
		on conflict (resource_id) do update set secret_value = excluded.secret_value
	`, legacyShared); err != nil {
		t.Fatalf("seed legacy personal secret: %v", err)
	}
	if err := resourceRepo.UpgradeSecretEncryption(ctx, secretCipher, authRepo); err != nil {
		t.Fatalf("slice-4 migration: %v", err)
	}
	var migrated string
	if err := pool.QueryRow(ctx, `select secret_value from resource_secrets where resource_id = 'legacy-personal'`).Scan(&migrated); err != nil {
		t.Fatalf("read migrated: %v", err)
	}
	if !strings.HasPrefix(migrated, "enc:v2:personal:owner:") {
		t.Fatalf("expected legacy personal secret converted to personal envelope, got %q", migrated)
	}
	if _, err := secretCipher.DecryptFromStorage(ctx, migrated); !errors.Is(err, ErrVaultLocked) {
		t.Fatalf("expected converted secret to be server-unreadable, got %v", err)
	}
	if reveal, err := service.Reveal(ctx, sessionUser, "legacy-personal"); err != nil || reveal.SecretValue != "pre-vault-personal-pw" {
		t.Fatalf("owner reveal of migrated secret: got %q err=%v", reveal.SecretValue, err)
	}
	// Rerun: converted rows are stable.
	if err := resourceRepo.UpgradeSecretEncryption(ctx, secretCipher, authRepo); err != nil {
		t.Fatalf("slice-4 migration rerun: %v", err)
	}
	var afterRerun string
	if err := pool.QueryRow(ctx, `select secret_value from resource_secrets where resource_id = 'legacy-personal'`).Scan(&afterRerun); err != nil {
		t.Fatalf("reread migrated: %v", err)
	}
	if afterRerun != migrated {
		t.Fatalf("expected rerun to leave converted personal row untouched")
	}

	// Update without resending the secret keeps the envelope intact.
	update := resourceToUpdateInput(resource)
	update.Name = "my-portal-login-renamed"
	update.SecretValue = ""
	if _, err := service.Update(ctx, sessionUser, resource.ID, update); err != nil {
		t.Fatalf("rename update: %v", err)
	}
	if reveal, err := service.Reveal(ctx, sessionUser, resource.ID); err != nil || reveal.SecretValue != "personal-pw-123" {
		t.Fatalf("reveal after rename: got %q err=%v", reveal.SecretValue, err)
	}
}

func TestAccountLifecycleEndToEnd(t *testing.T) {
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

	authRepo := auth.NewRepository(pool)
	authService := auth.NewService(authRepo, auth.ModeDev, "")
	secretCipher := NewSecretCipher("verify-test-key-000000000000000000000000")
	service := NewService(NewRepository(pool), &captureAuditLogger{}, fakeKeyVaultResolver{}, nil, nil, secretCipher, authRepo)

	adminUser := auth.User{ID: "lifecycle-admin", Name: "Lifecycle Admin", IsAdmin: true}

	// Invited user: account exists, login impossible, no vault, no password
	// known to any admin.
	created, err := authService.CreateUser(ctx, auth.CreateUserInput{
		Username:    "invited-user",
		DisplayName: "Invited User",
		Email:       "invited@example.com",
		Invite:      true,
		IsAdmin:     true,
	})
	if err != nil {
		t.Fatalf("create invited user: %v", err)
	}
	if _, err := authService.Login(ctx, "invited-user", "anything-at-all"); err == nil {
		t.Fatalf("expected login to fail before invite acceptance")
	}
	if pub, err := authRepo.VaultPublicKey(ctx, created.ID); err != nil || pub != nil {
		t.Fatalf("expected no vault before acceptance, got %d bytes err=%v", len(pub), err)
	}

	invite, err := authService.IssueUserInvite(ctx, adminUser, created.ID, auth.InvitePurposeInvite)
	if err != nil {
		t.Fatalf("issue invite: %v", err)
	}

	// User accepts: sets their own password, vault is born, session unlocked.
	const ownPassword = "user-chosen-password-1"
	accepted, err := authService.AcceptInvite(ctx, invite.Token, ownPassword)
	if err != nil {
		t.Fatalf("accept invite: %v", err)
	}
	if len(accepted.User.VaultPrivateKey) != 32 {
		t.Fatalf("expected unlocked vault after acceptance")
	}
	if _, err := authService.AcceptInvite(ctx, invite.Token, "second-use-password"); err == nil {
		t.Fatalf("expected invite to be single-use")
	}

	personal, err := service.Create(ctx, accepted.User, CreateResourceInput{
		Name:          "lifecycle-personal",
		Type:          TypeSharedSecret,
		Personal:      true,
		Owner:         "Invited User",
		Username:      "invited@example.com",
		SecretMode:    SecretModeInline,
		SecretValue:   "invited-personal-pw",
		RevealAllowed: true,
	})
	if err != nil {
		t.Fatalf("create personal: %v", err)
	}

	// Password change: login password rotates, vault and secrets survive.
	const newPassword = "user-rotated-password-2"
	if err := authService.ChangeOwnPassword(ctx, accepted.User, ownPassword, newPassword); err != nil {
		t.Fatalf("change password: %v", err)
	}
	if err := authService.ChangeOwnPassword(ctx, accepted.User, "wrong-current", "whatever-123"); err == nil {
		t.Fatalf("expected wrong current password to be rejected")
	}
	if _, err := authService.Login(ctx, "invited-user", ownPassword); err == nil {
		t.Fatalf("expected old password to stop working")
	}
	relogin, err := authService.Login(ctx, "invited-user", newPassword)
	if err != nil {
		t.Fatalf("login with new password: %v", err)
	}
	freshUser, err := authRepo.UserByToken(ctx, relogin.Token)
	if err != nil || len(freshUser.VaultPrivateKey) != 32 {
		t.Fatalf("expected vault unlocked after password change + relogin, err=%v", err)
	}
	if reveal, err := service.Reveal(ctx, freshUser, personal.ID); err != nil || reveal.SecretValue != "invited-personal-pw" {
		t.Fatalf("expected personal secret to survive password change: got %q err=%v", reveal.SecretValue, err)
	}

	// Admin-forced reset: vault + personal secrets destroyed, sessions dead.
	resetInvite, err := authService.ResetUserPassword(ctx, adminUser, created.ID)
	if err != nil {
		t.Fatalf("reset password: %v", err)
	}
	if resetInvite.PersonalResourcesDeleted != 1 {
		t.Fatalf("expected 1 personal resource destroyed, got %d", resetInvite.PersonalResourcesDeleted)
	}
	if _, err := authRepo.UserByToken(ctx, relogin.Token); err == nil {
		t.Fatalf("expected sessions to be revoked by reset")
	}
	if _, err := authService.Login(ctx, "invited-user", newPassword); err == nil {
		t.Fatalf("expected login to be locked until reset link used")
	}
	if pub, err := authRepo.VaultPublicKey(ctx, created.ID); err != nil || pub != nil {
		t.Fatalf("expected vault destroyed by reset, got %d bytes err=%v", len(pub), err)
	}

	// Reset link works once; a brand-new vault is created.
	restored, err := authService.AcceptInvite(ctx, resetInvite.Token, "after-reset-password-3")
	if err != nil {
		t.Fatalf("accept reset invite: %v", err)
	}
	if len(restored.User.VaultPrivateKey) != 32 {
		t.Fatalf("expected fresh vault after reset acceptance")
	}
	if _, err := service.Reveal(ctx, restored.User, personal.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected old personal resource to be gone, got %v", err)
	}
}
