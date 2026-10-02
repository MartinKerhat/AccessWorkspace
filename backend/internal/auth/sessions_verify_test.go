package auth

// End-to-end verification of session listing and revocation against a
// throwaway database. Runs only when VERIFY_DATABASE_URL is set (same pattern
// as the lockout and vault-method verify tests).

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/MartinKerhat/AccessWorkspace/backend/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSessionRevocationEndToEnd(t *testing.T) {
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
	hash, err := HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	for _, id := range []string{"sess-a", "sess-b"} {
		if _, err := pool.Exec(ctx, `
			insert into app_users (id, username, display_name, email, password_hash)
			values ($1, $1, $1, $1 || '@example.com', $2)
			on conflict (id) do update set password_hash = excluded.password_hash
		`, id, hash); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
		if _, err := repo.DeleteAllSessions(ctx, id); err != nil {
			t.Fatalf("reset sessions %s: %v", id, err)
		}
	}

	clientCtx := WithSessionClient(ctx, SessionClient{IP: "10.1.2.3", UserAgent: "Mozilla/5.0 (Windows NT 10.0) Chrome/129.0 Safari/537.36 Edg/129.0"})
	tokA1, err := repo.CreateSession(clientCtx, "sess-a", time.Hour)
	if err != nil {
		t.Fatalf("session a1: %v", err)
	}
	tokA2, err := repo.CreateSession(ctx, "sess-a", time.Hour)
	if err != nil {
		t.Fatalf("session a2: %v", err)
	}
	extA, err := repo.UpsertBrowserExtensionSession(clientCtx, "sess-a", "install-a", time.Hour, nil)
	if err != nil {
		t.Fatalf("extension a: %v", err)
	}
	tokB, err := repo.CreateSession(ctx, "sess-b", time.Hour)
	if err != nil {
		t.Fatalf("session b: %v", err)
	}

	// Listing is user-scoped and marks the current one; client info captured.
	sessions, err := repo.ListSessions(ctx, "sess-a", tokA1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(sessions) != 3 {
		t.Fatalf("expected 3 sessions for a, got %d", len(sessions))
	}
	var current *SessionInfo
	kinds := map[string]int{}
	for i := range sessions {
		kinds[sessions[i].Kind]++
		if sessions[i].Current {
			current = &sessions[i]
		}
	}
	if kinds[SessionKindWeb] != 2 || kinds[SessionKindExtension] != 1 {
		t.Fatalf("unexpected kinds %v", kinds)
	}
	if current == nil || current.IP != "10.1.2.3" || current.Client != "Edge on Windows" {
		t.Fatalf("current session not recognised or client not captured: %+v", current)
	}

	// Cross-user revoke by id: user B cannot touch A's session.
	if _, _, err := repo.DeleteSessionByID(ctx, "sess-b", current.ID, tokB); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for foreign session id, got %v", err)
	}
	if _, err := repo.UserByToken(ctx, tokA1); err != nil {
		t.Fatalf("a1 must still be valid after foreign revoke attempt: %v", err)
	}

	// Revoke others keeps the current session, kills the other web + extension.
	revoked, err := repo.DeleteOtherSessions(ctx, "sess-a", tokA1)
	if err != nil {
		t.Fatalf("revoke others: %v", err)
	}
	if revoked != 2 {
		t.Fatalf("expected 2 revoked, got %d", revoked)
	}
	if _, err := repo.UserByToken(ctx, tokA1); err != nil {
		t.Fatalf("current session must survive: %v", err)
	}
	if _, err := repo.UserByToken(ctx, tokA2); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("a2 should be gone, got %v", err)
	}
	if _, err := repo.UserByToken(ctx, extA); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("extension session should be gone, got %v", err)
	}
	if _, err := repo.UserByToken(ctx, tokB); err != nil {
		t.Fatalf("user b untouched: %v", err)
	}

	// Revoking the current session by id reports wasCurrent.
	sessions, _ = repo.ListSessions(ctx, "sess-a", tokA1)
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session left, got %d", len(sessions))
	}
	kind, wasCurrent, err := repo.DeleteSessionByID(ctx, "sess-a", sessions[0].ID, tokA1)
	if err != nil || kind != SessionKindWeb || !wasCurrent {
		t.Fatalf("expected current web session deleted, got kind=%q current=%v err=%v", kind, wasCurrent, err)
	}

	// Password change signs out every other session.
	svc := NewService(repo, ModeDev, "[]")
	keep, err := repo.CreateSession(ctx, "sess-a", time.Hour)
	if err != nil {
		t.Fatalf("keep: %v", err)
	}
	other, err := repo.CreateSession(ctx, "sess-a", time.Hour)
	if err != nil {
		t.Fatalf("other: %v", err)
	}
	user, err := repo.UserByToken(ctx, keep)
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	n, err := svc.ChangeOwnPassword(ctx, user, keep, "correct-horse-battery", "new-horse-battery-9")
	if err != nil {
		t.Fatalf("change password: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 other session revoked on password change, got %d", n)
	}
	if _, err := repo.UserByToken(ctx, keep); err != nil {
		t.Fatalf("session making the change must survive: %v", err)
	}
	if _, err := repo.UserByToken(ctx, other); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("other session should be gone after password change, got %v", err)
	}
}
