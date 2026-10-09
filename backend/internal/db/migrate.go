package db

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"

	"github.com/MartinKerhat/AccessWorkspace/backend/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Migration files are named <YYYYMMDDHHMM>_<description>.sql (UTC stamp).
// Timestamps cannot collide across branches the way hand-picked sequence
// numbers did, and lexical order is chronological. The ledger table records
// the file name, so a file must never be renamed once it has been applied
// anywhere.
var migrationNamePattern = regexp.MustCompile(`^(\d{12})_[a-z0-9_]+\.sql$`)

// BaselineMigration replaces the historical 001–019 series. A database whose
// ledger still lists any of those legacy names is one that was migrated by
// the old series — the schema is identical to what the baseline creates — so
// the runner swaps the legacy rows for this one name instead of re-applying.
const BaselineMigration = "202610071500_baseline.sql"

var legacyLedgerPattern = regexp.MustCompile(`^\d{3}_`)

// ValidateMigrationNames checks the embedded folder: every file matches the
// naming scheme, no two files share a timestamp, and the baseline exists.
// Used at startup and by the unit test, so a bad name fails in `go test`
// long before a deploy.
func ValidateMigrationNames(names []string) error {
	seen := map[string]string{}
	foundBaseline := false
	for _, name := range names {
		match := migrationNamePattern.FindStringSubmatch(name)
		if match == nil {
			return fmt.Errorf("migration %q: name must be <YYYYMMDDHHMM>_<description>.sql (lower-case, underscores); the legacy NNN_ numbering is closed", name)
		}
		if other, dup := seen[match[1]]; dup {
			return fmt.Errorf("migrations %q and %q share the timestamp %s — re-stamp one of them", other, name, match[1])
		}
		seen[match[1]] = name
		if name == BaselineMigration {
			foundBaseline = true
		}
	}
	if !foundBaseline {
		return fmt.Errorf("baseline migration %q is missing", BaselineMigration)
	}
	return nil
}

// MigrationFileNames lists the embedded migration files in apply order.
func MigrationFileNames() ([]string, error) {
	entries, err := migrations.Files.ReadDir(".")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}

func RunMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, `
		create table if not exists schema_migrations (
			version text primary key,
			applied_at timestamptz not null default now()
		)
	`); err != nil {
		return err
	}

	names, err := MigrationFileNames()
	if err != nil {
		return err
	}
	if err := ValidateMigrationNames(names); err != nil {
		return err
	}

	if err := adoptLegacyLedger(ctx, pool); err != nil {
		return err
	}

	for _, name := range names {
		var exists bool
		if err := pool.QueryRow(ctx, `select exists(select 1 from schema_migrations where version = $1)`, name).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}

		sql, err := migrations.Files.ReadFile(name)
		if err != nil {
			return err
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}

		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply %s: %w", name, err)
		}

		if _, err := tx.Exec(ctx, `insert into schema_migrations (version) values ($1)`, name); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}

		if err := tx.Commit(ctx); err != nil {
			return err
		}
		log.Printf("migration applied: %s", name)
	}

	return nil
}

// adoptLegacyLedger handles databases migrated by the historical 001–019
// series: their ledger rows are replaced by the baseline's name, in one
// transaction, without touching the schema. A fresh database has no legacy
// rows and simply gets the baseline applied by the normal loop.
func adoptLegacyLedger(ctx context.Context, pool *pgxpool.Pool) error {
	rows, err := pool.Query(ctx, `select version from schema_migrations where version ~ '^\d{3}_'`)
	if err != nil {
		return err
	}
	var legacy []string
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			rows.Close()
			return err
		}
		legacy = append(legacy, version)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(legacy) == 0 {
		return nil
	}
	for _, version := range legacy {
		if !legacyLedgerPattern.MatchString(version) {
			return fmt.Errorf("unexpected legacy ledger entry %q", version)
		}
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	if _, err := tx.Exec(ctx, `delete from schema_migrations where version ~ '^\d{3}_'`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		insert into schema_migrations (version) values ($1)
		on conflict (version) do nothing
	`, BaselineMigration); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	log.Printf("migration ledger: adopted %d legacy entries as %s", len(legacy), BaselineMigration)
	return nil
}
