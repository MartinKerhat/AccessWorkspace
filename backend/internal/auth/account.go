package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

// Account lifecycle: self-service password change, one-time invite links,
// and admin-forced resets. These are part of the personal-vault design:
//
//   - password change (current password known): the vault private key is
//     re-wrapped under the new password — personal secrets survive.
//   - invite: the admin creates the account WITHOUT a password; the user
//     sets it via a one-time link, and the vault is born from a password no
//     admin has ever seen.
//   - reset: the admin cannot recover the vault (that is the point), so a
//     forced reset DESTROYS the vault and the user's personal secrets, then
//     issues a reset link. Explicit and loud, by design.

// invitePendingPasswordHash is stored for accounts awaiting an invite. It is
// not a valid bcrypt hash, so Authenticate can never match it.
const invitePendingPasswordHash = "!invite-pending"

const (
	InvitePurposeInvite = "invite"
	InvitePurposeReset  = "reset"

	inviteTTL = 7 * 24 * time.Hour
)

type UserInvite struct {
	Token                    string    `json:"token"`
	UserID                   string    `json:"userId"`
	Purpose                  string    `json:"purpose"`
	ExpiresAt                time.Time `json:"expiresAt"`
	PersonalResourcesDeleted int       `json:"personalResourcesDeleted,omitempty"`
	// EmailSent reports whether the link was emailed to the user (best
	// effort, requires configured SMTP). The link is always returned for
	// manual sharing regardless.
	EmailSent bool `json:"emailSent"`
}

// InviteMailer delivers invite/reset links by email. Implemented by the
// notifications service; optional — without it links are share-manually only.
type InviteMailer interface {
	SendPlainEmail(ctx context.Context, toEmail, subject, body string) error
}

// ConfigureInviteDelivery wires optional email delivery for invite links.
// baseURL is the public frontend URL the links are built against.
func (s *Service) ConfigureInviteDelivery(mailer InviteMailer, baseURL string) {
	s.inviteMailer = mailer
	s.inviteBaseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
}

func (s *Service) emailInvite(ctx context.Context, user User, invite UserInvite) bool {
	if s.inviteMailer == nil || s.inviteBaseURL == "" || strings.TrimSpace(user.Email) == "" {
		return false
	}
	link := s.inviteBaseURL + "/?invite=" + invite.Token
	subject := "Access Workspace invitation"
	intro := "An account was created for you in Access Workspace."
	if invite.Purpose == InvitePurposeReset {
		subject = "Access Workspace password reset"
		intro = "Your Access Workspace password was reset by an administrator."
	}
	body := fmt.Sprintf(
		"Hi %s,\n\n%s Set your password using this one-time link:\n\n%s\n\nThe link can be used once and expires %s.\n",
		user.Name, intro, link, invite.ExpiresAt.Local().Format("2006-01-02 15:04"),
	)
	return s.inviteMailer.SendPlainEmail(ctx, user.Email, subject, body) == nil
}

// ChangeOwnPassword verifies the current password, stores the new one, and
// re-wraps the vault's password unlock so personal secrets survive.
func (s *Service) ChangeOwnPassword(ctx context.Context, user User, currentPassword, newPassword string) error {
	currentPassword = strings.TrimSpace(currentPassword)
	newPassword = strings.TrimSpace(newPassword)
	if len(newPassword) < 8 {
		return fmt.Errorf("%w: new password must be at least 8 characters", ErrInvalidInput)
	}
	if err := s.repo.verifyPassword(ctx, user.ID, currentPassword); err != nil {
		return err
	}

	// Prefer the session-carried vault key; fall back to unlocking with the
	// (just verified) current password.
	privateKey := user.VaultPrivateKey
	if len(privateKey) == 0 {
		unlocked, err := s.repo.unlockVaultWithPassword(ctx, user.ID, currentPassword)
		if err != nil {
			return err
		}
		privateKey = unlocked
	}

	if err := s.repo.setPasswordHash(ctx, user.ID, newPassword); err != nil {
		return err
	}
	if len(privateKey) > 0 {
		return s.repo.rewrapPasswordUnlock(ctx, user.ID, privateKey, newPassword)
	}
	// No vault could be opened (none exists, or its wrap predates unknown
	// history): make sure one exists under the new password going forward.
	_, err := s.repo.EnsureLocalUserVault(ctx, user.ID, newPassword)
	return err
}

// IssueUserInvite (re)creates the one-time setup link for a user. Any
// previous link for the user is invalidated.
func (s *Service) IssueUserInvite(ctx context.Context, actor User, userID string, purpose string) (UserInvite, error) {
	if purpose != InvitePurposeInvite && purpose != InvitePurposeReset {
		return UserInvite{}, fmt.Errorf("%w: invalid invite purpose", ErrInvalidInput)
	}
	target, err := s.repo.userByID(ctx, userID)
	if err != nil {
		return UserInvite{}, err
	}
	token, expiresAt, err := s.repo.createUserInvite(ctx, userID, purpose, actor.ID, inviteTTL)
	if err != nil {
		return UserInvite{}, err
	}
	invite := UserInvite{Token: token, UserID: userID, Purpose: purpose, ExpiresAt: expiresAt}
	invite.EmailSent = s.emailInvite(ctx, target, invite)
	return invite, nil
}

// ResetUserPassword is the admin-forced reset: destroys the user's vault and
// personal secrets (unrecoverable without the owner — by design), kills all
// their sessions, locks the account, and issues a one-time reset link.
func (s *Service) ResetUserPassword(ctx context.Context, actor User, userID string) (UserInvite, error) {
	target, err := s.repo.userByID(ctx, userID)
	if err != nil {
		return UserInvite{}, err
	}
	personalDeleted, err := s.repo.destroyVaultForReset(ctx, target.ID)
	if err != nil {
		return UserInvite{}, err
	}
	invite, err := s.IssueUserInvite(ctx, actor, target.ID, InvitePurposeReset)
	if err != nil {
		return UserInvite{}, err
	}
	invite.PersonalResourcesDeleted = personalDeleted
	return invite, nil
}

// AcceptInvite consumes a one-time link, sets the user's password, creates
// their vault from it, and signs them in.
func (s *Service) AcceptInvite(ctx context.Context, token, password string) (LoginResult, error) {
	password = strings.TrimSpace(password)
	if len(password) < 8 {
		return LoginResult{}, fmt.Errorf("%w: password must be at least 8 characters", ErrInvalidInput)
	}
	userID, _, err := s.repo.consumeUserInvite(ctx, token)
	if err != nil {
		return LoginResult{}, err
	}
	if err := s.repo.setPasswordHash(ctx, userID, password); err != nil {
		return LoginResult{}, err
	}
	user, err := s.repo.userByID(ctx, userID)
	if err != nil {
		return LoginResult{}, err
	}
	result, err := s.issueSession(ctx, user, s.mode, false)
	if err != nil {
		return LoginResult{}, err
	}
	vaultKey, err := s.repo.EnsureLocalUserVault(ctx, userID, password)
	if err != nil {
		return LoginResult{}, err
	}
	if len(vaultKey) > 0 {
		if err := s.repo.attachVaultKeyToSession(ctx, "auth_sessions", result.Token, vaultKey); err != nil {
			return LoginResult{}, err
		}
		result.User.VaultPrivateKey = vaultKey
	}
	return result, nil
}

// GetVaultStatus reports the user's vault state; unlocked reflects whether
// this request's session already carries the key.
func (s *Service) GetVaultStatus(ctx context.Context, user User) (VaultStatus, error) {
	status, err := s.repo.VaultStatus(ctx, user.ID)
	if err != nil {
		return VaultStatus{}, err
	}
	status.Unlocked = len(user.VaultPrivateKey) > 0
	descriptors, err := s.repo.ListPasskeyDescriptors(ctx, user.ID)
	if err != nil {
		return VaultStatus{}, err
	}
	status.Passkeys = descriptors
	return status, nil
}

// LockVault re-locks the vault for the current session (clears the key).
func (s *Service) LockVault(ctx context.Context, token string) error {
	return s.repo.clearSessionVaultKey(ctx, token)
}

// SetupVaultWithPasskey creates a first vault wrapped under a WebAuthn PRF
// secret (Windows Hello / Touch ID) — no passphrase — and unlocks the session.
func (s *Service) SetupVaultWithPasskey(ctx context.Context, user User, token, credentialID, prfSalt, prfSecret string) error {
	privateKey, err := s.repo.SetupVaultWithPasskey(ctx, user.ID, credentialID, prfSalt, prfSecret)
	if err != nil {
		return err
	}
	return s.repo.attachVaultKeyToCurrentSession(ctx, token, privateKey)
}

// UnlockVaultWithPasskey opens the vault with a credential's PRF output and
// attaches the key to the current session.
func (s *Service) UnlockVaultWithPasskey(ctx context.Context, user User, token, credentialID, prfSecret string) error {
	privateKey, err := s.repo.UnlockVaultWithPasskey(ctx, user.ID, credentialID, prfSecret)
	if err != nil {
		return err
	}
	if len(privateKey) == 0 {
		return ErrUnauthenticated
	}
	return s.repo.attachVaultKeyToCurrentSession(ctx, token, privateKey)
}

// AddVaultPasskey registers an extra passkey against an unlocked vault.
func (s *Service) AddVaultPasskey(ctx context.Context, user User, credentialID, prfSalt, prfSecret string) error {
	if len(user.VaultPrivateKey) == 0 {
		return ErrVaultLocked
	}
	return s.repo.AddPasskeyMethod(ctx, user.ID, user.VaultPrivateKey, credentialID, prfSalt, prfSecret)
}

// SetupVault creates a first vault for a user who has none (SSO users on
// first personal-secret use) and unlocks it for the current session.
func (s *Service) SetupVault(ctx context.Context, user User, token, passphrase string) error {
	if len(strings.TrimSpace(passphrase)) < 8 {
		return fmt.Errorf("%w: passphrase must be at least 8 characters", ErrInvalidInput)
	}
	privateKey, err := s.repo.SetupVaultWithPassphrase(ctx, user.ID, passphrase)
	if err != nil {
		return err
	}
	return s.repo.attachVaultKeyToCurrentSession(ctx, token, privateKey)
}

// UnlockVault opens the vault with a passphrase (or login password) and
// attaches the key to the current session. Wrong secret → ErrUnauthenticated.
func (s *Service) UnlockVault(ctx context.Context, user User, token, secret string) error {
	privateKey, err := s.repo.UnlockVaultWithSecret(ctx, user.ID, secret)
	if err != nil {
		return err
	}
	if len(privateKey) == 0 {
		return ErrUnauthenticated
	}
	return s.repo.attachVaultKeyToCurrentSession(ctx, token, privateKey)
}

// AddVaultPassphrase adds a passphrase unlock to an already-unlocked vault.
func (s *Service) AddVaultPassphrase(ctx context.Context, user User, passphrase string) error {
	if len(strings.TrimSpace(passphrase)) < 8 {
		return fmt.Errorf("%w: passphrase must be at least 8 characters", ErrInvalidInput)
	}
	if len(user.VaultPrivateKey) == 0 {
		return ErrVaultLocked
	}
	return s.repo.AddPassphraseMethod(ctx, user.ID, user.VaultPrivateKey, passphrase)
}

func (r *Repository) verifyPassword(ctx context.Context, userID, password string) error {
	var passwordHash string
	err := r.db.QueryRow(ctx, `select password_hash from app_users where id = $1`, strings.TrimSpace(userID)).Scan(&passwordHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUnauthenticated
		}
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) != nil {
		return ErrUnauthenticated
	}
	return nil
}

func (r *Repository) setPasswordHash(ctx context.Context, userID, password string) error {
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		update app_users set password_hash = $2, updated_at = now() where id = $1
	`, strings.TrimSpace(userID), hash)
	return err
}

// rewrapPasswordUnlock replaces the password unlock method with a wrap under
// the new password (fresh salt). The private key itself never changes, so
// existing sessions and all personal secrets remain valid.
func (r *Repository) rewrapPasswordUnlock(ctx context.Context, userID string, privateKey []byte, newPassword string) error {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return err
	}
	wrapped, err := vaultSeal(derivePasswordWrapKey(newPassword, salt), privateKey)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		insert into user_vault_unlocks (user_id, method, label, wrapped_private_key, salt)
		values ($1, $2, '', $3, $4)
		on conflict (user_id, method, label) do update
		set wrapped_private_key = excluded.wrapped_private_key,
			salt = excluded.salt,
			updated_at = now()
	`, strings.TrimSpace(userID), vaultUnlockMethodPassword, wrapped, base64.StdEncoding.EncodeToString(salt))
	return err
}

func (r *Repository) createUserInvite(ctx context.Context, userID, purpose, createdBy string, ttl time.Duration) (string, time.Time, error) {
	token := uuid.NewString()
	expiresAt := time.Now().UTC().Add(ttl)
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return "", time.Time{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	if _, err := tx.Exec(ctx, `delete from user_invites where user_id = $1`, strings.TrimSpace(userID)); err != nil {
		return "", time.Time{}, err
	}
	if _, err := tx.Exec(ctx, `
		insert into user_invites (token, user_id, purpose, created_by, expires_at)
		values ($1, $2, $3, $4, $5)
	`, hashToken(token), strings.TrimSpace(userID), purpose, strings.TrimSpace(createdBy), expiresAt); err != nil {
		return "", time.Time{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", time.Time{}, err
	}
	return token, expiresAt, nil
}

func (r *Repository) consumeUserInvite(ctx context.Context, token string) (string, string, error) {
	var userID, purpose string
	err := r.db.QueryRow(ctx, `
		delete from user_invites
		where token = $1 and expires_at > now()
		returning user_id, purpose
	`, hashToken(strings.TrimSpace(token))).Scan(&userID, &purpose)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", fmt.Errorf("%w: invite link is invalid or expired", ErrUnauthenticated)
		}
		return "", "", err
	}
	return userID, purpose, nil
}

// destroyVaultForReset removes everything only the user's password could
// open: the vault, its unlock methods, their personal resources, and every
// live session. The account remains, locked behind the reset link.
func (r *Repository) destroyVaultForReset(ctx context.Context, userID string) (int, error) {
	userID = strings.TrimSpace(userID)
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	tag, err := tx.Exec(ctx, `delete from resources where owner_user_id = $1 and personal = true`, userID)
	if err != nil {
		return 0, err
	}
	personalDeleted := int(tag.RowsAffected())

	if _, err := tx.Exec(ctx, `delete from connection_user_password_overrides where user_id = $1`, userID); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `delete from user_vault_unlocks where user_id = $1`, userID); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `delete from user_vaults where user_id = $1`, userID); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `delete from auth_sessions where user_id = $1`, userID); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `delete from browser_extension_sessions where user_id = $1`, userID); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `delete from browser_extension_connect_tokens where user_id = $1`, userID); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `
		update app_users set password_hash = $2, updated_at = now() where id = $1
	`, userID, invitePendingPasswordHash); err != nil {
		return 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return personalDeleted, nil
}
