package auth

// End-to-end verification of brute-force lockout against a throwaway database.
// Runs only when VERIFY_DATABASE_URL is set.

import (
	"context"
	"errors"
	"os"
	"testing"

	"access-workspace/backend/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLoginLockoutEndToEnd(t *testing.T) {
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
	if _, err := pool.Exec(ctx, `
		insert into app_users (id, username, display_name, email, password_hash)
		values ('lock-user', 'lock-user', 'Lock User', 'lock@example.com', $1)
		on conflict (id) do update set password_hash = excluded.password_hash,
			failed_login_attempts = 0, locked_until = null
	`, hash); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Wrong password up to the threshold returns ErrUnauthenticated.
	for i := 0; i < maxFailedLogins; i++ {
		if _, err := repo.Authenticate(ctx, "lock-user", "wrong"); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("attempt %d: expected ErrUnauthenticated, got %v", i+1, err)
		}
	}

	// Now locked: even the CORRECT password is refused with ErrLockedOut.
	if _, err := repo.Authenticate(ctx, "lock-user", "correct-horse-battery"); !errors.Is(err, ErrLockedOut) {
		t.Fatalf("expected ErrLockedOut after threshold, got %v", err)
	}

	// Simulate the cooldown elapsing; the correct password then works and
	// clears the counter.
	if _, err := pool.Exec(ctx, `update app_users set locked_until = now() - interval '1 minute' where id = 'lock-user'`); err != nil {
		t.Fatalf("expire lock: %v", err)
	}
	if _, err := repo.Authenticate(ctx, "lock-user", "correct-horse-battery"); err != nil {
		t.Fatalf("expected success after cooldown, got %v", err)
	}
	var attempts int
	var lockedUntil *string
	if err := pool.QueryRow(ctx, `select failed_login_attempts, locked_until::text from app_users where id = 'lock-user'`).Scan(&attempts, &lockedUntil); err != nil {
		t.Fatalf("read counters: %v", err)
	}
	if attempts != 0 || lockedUntil != nil {
		t.Fatalf("expected counters cleared on success, got attempts=%d lockedUntil=%v", attempts, lockedUntil)
	}
}
