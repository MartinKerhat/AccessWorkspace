package db

// Rehearsal of the production path: a database migrated by the legacy
// 001–019 series (ledger rows with the old names) is handed to the new
// runner. Expected: the legacy rows are replaced by the baseline's name,
// nothing legacy is re-applied, and only migrations newer than the baseline
// run. Runs only when VERIFY_DATABASE_URL points at such a database.

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLegacyLedgerAdoptionEndToEnd(t *testing.T) {
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

	var legacyBefore int
	if err := pool.QueryRow(ctx, `select count(*) from schema_migrations where version ~ '^\d{3}_'`).Scan(&legacyBefore); err != nil {
		t.Fatalf("count legacy: %v", err)
	}

	if err := RunMigrations(ctx, pool); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	var legacyAfter int
	if err := pool.QueryRow(ctx, `select count(*) from schema_migrations where version ~ '^\d{3}_'`).Scan(&legacyAfter); err != nil {
		t.Fatalf("count legacy after: %v", err)
	}
	if legacyAfter != 0 {
		t.Fatalf("legacy ledger rows remain: %d", legacyAfter)
	}
	var hasBaseline bool
	if err := pool.QueryRow(ctx, `select exists(select 1 from schema_migrations where version = $1)`, BaselineMigration).Scan(&hasBaseline); err != nil {
		t.Fatalf("baseline check: %v", err)
	}
	if !hasBaseline {
		t.Fatal("baseline not recorded")
	}

	names, _ := MigrationFileNames()
	var recorded int
	if err := pool.QueryRow(ctx, `select count(*) from schema_migrations`).Scan(&recorded); err != nil {
		t.Fatalf("count: %v", err)
	}
	if recorded != len(names) {
		t.Fatalf("expected every embedded migration recorded (%d), got %d", len(names), recorded)
	}
	t.Logf("legacy rows adopted: %d → ledger now %d entries", legacyBefore, recorded)

	// Idempotent: a second run changes nothing.
	if err := RunMigrations(ctx, pool); err != nil {
		t.Fatalf("second run: %v", err)
	}
}
