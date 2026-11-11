package auth

// End-to-end verification of local sign-in: email login, the admin reset
// round trip, whitespace handling, and refusal of Microsoft-provisioned rows.
// Runs only when VERIFY_DATABASE_URL is set.

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/MartinKerhat/AccessWorkspace/backend/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLocalLoginEndToEnd(t *testing.T) {
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
	svc := NewService(repo, ModeDev, "")
	admin := User{ID: "login-verify-admin", Name: "Admin", IsAdmin: true}
	suffix := uuid.NewString()[:8]

	// Invited user: never typed a username, then the admin forces a reset.
	username := "Login-" + suffix
	email := "login-" + suffix + "@example.com"
	created, err := svc.CreateUser(ctx, CreateUserInput{Username: username, DisplayName: "Login User", Email: email, Invite: true})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	userID := created.ID
	if created.Username != strings.ToLower(username) {
		t.Fatalf("user detail username = %q, want %q", created.Username, strings.ToLower(username))
	}
	invite, err := svc.ResetUserPassword(ctx, admin, userID)
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	if invite.Username != strings.ToLower(username) {
		t.Fatalf("reset link username = %q, want %q", invite.Username, strings.ToLower(username))
	}
	// Pasted with a trailing newline: stored trimmed.
	accepted, err := svc.AcceptInvite(ctx, invite.Token, "fresh-password-1\n")
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if err := svc.Logout(ctx, accepted.Token); err != nil {
		t.Fatalf("logout: %v", err)
	}

	for _, tc := range []struct{ name, login, password string }{
		{"username", username, "fresh-password-1"},
		{"username any case", "LOGIN-" + suffix, "fresh-password-1"},
		{"email", email, "fresh-password-1"},
		{"email any case", "LOGIN-" + suffix + "@EXAMPLE.com", "fresh-password-1"},
		{"same paste with newline", username, "fresh-password-1\n"},
	} {
		result, err := svc.Login(ctx, tc.login, tc.password)
		if err != nil {
			t.Fatalf("%s: login failed: %v", tc.name, err)
		}
		if result.User.ID != userID {
			t.Fatalf("%s: signed in as %q, want %q", tc.name, result.User.ID, userID)
		}
		if len(result.User.VaultPrivateKey) == 0 {
			t.Fatalf("%s: vault not unlocked by the login password", tc.name)
		}
	}
	if _, err := svc.Login(ctx, email, "wrong-password"); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("wrong password: expected ErrUnauthenticated, got %v", err)
	}
	if _, err := pool.Exec(ctx, `update app_users set failed_login_attempts = 0 where id = $1`, userID); err != nil {
		t.Fatalf("reset counter: %v", err)
	}

	// An email shared by two local accounts is ambiguous and never picks one.
	shared := "shared-" + suffix + "@example.com"
	for _, name := range []string{"shared-a-" + suffix, "shared-b-" + suffix} {
		if _, err := svc.CreateUser(ctx, CreateUserInput{Username: name, DisplayName: name, Email: shared, Password: "shared-password-1"}); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}
	if _, err := svc.Login(ctx, shared, "shared-password-1"); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("ambiguous email: expected ErrUnauthenticated, got %v", err)
	}
	if _, err := svc.Login(ctx, "shared-a-"+suffix, "shared-password-1"); err != nil {
		t.Fatalf("username still works when email is ambiguous: %v", err)
	}

	// Microsoft-provisioned rows hold a hash of a public placeholder; neither
	// their username nor their email may sign in locally with it.
	entraEmail := "entra-" + suffix + "@example.com"
	if err := repo.UpsertExternalUser(ctx, User{ID: "oid-" + suffix, Name: "Entra User", Email: entraEmail}); err != nil {
		t.Fatalf("upsert external: %v", err)
	}
	if detail, err := svc.GetUserAccess(ctx, "oid-"+suffix); err != nil || detail.Username != "" {
		t.Fatalf("external account detail: username %q, err %v; want no username", detail.Username, err)
	}
	for _, login := range []string{externalUsernamePrefix + "oid-" + suffix, entraEmail} {
		if _, err := svc.Login(ctx, login, externalPasswordPlaceholder); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("external account %q: expected ErrUnauthenticated, got %v", login, err)
		}
	}
	if _, err := svc.CreateUser(ctx, CreateUserInput{Username: "Entra:spoof-" + suffix, DisplayName: "Spoof", Email: "spoof@example.com", Password: "spoof-password-1"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("reserved prefix: expected ErrInvalidInput, got %v", err)
	}
}
