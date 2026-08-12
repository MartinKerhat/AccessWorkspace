package resources

// Regression cover for the Key Vault / app registration sync jobs, which call
// the repository directly and therefore never run normalizeInput: an unset
// AllowedUsers list used to reach Postgres as NULL and break the not-null
// constraint on resources.allowed_users. Runs only when VERIFY_DATABASE_URL
// points at a throwaway database.

import (
	"context"
	"os"
	"testing"
	"time"

	"access-workspace/backend/internal/db"
	"access-workspace/backend/internal/keyvault"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSyncCreateWithoutSharingListsVerify(t *testing.T) {
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
	now := time.Now().UTC()
	input := autoImportCreateInput(KeyVaultSyncSourceConfig{
		VaultURL:     "https://kvinsio.vault.azure.net",
		DefaultOwner: "Sync Owner",
	}, keyvault.SecretItem{
		ID:        "https://kvinsio.vault.azure.net/secrets/sharing-list-regression/1",
		Name:      "sharing-list-regression",
		VaultName: "kvinsio",
		Version:   "1",
		Enabled:   true,
	}, now)
	if input.AllowedUsers != nil {
		t.Fatalf("expected the auto-import input to leave AllowedUsers unset for this check")
	}

	created, err := repo.Create(ctx, input)
	if err != nil {
		t.Fatalf("auto-import create with unset sharing lists: %v", err)
	}
	defer repo.Delete(ctx, created.ID)

	if created.AllowedUsers == nil || len(created.AllowedUsers) != 0 {
		t.Fatalf("expected an empty allowed_users array, got %#v", created.AllowedUsers)
	}
	if created.AllowedGroups == nil || len(created.AllowedGroups) != 0 {
		t.Fatalf("expected an empty allowed_groups array, got %#v", created.AllowedGroups)
	}

	// The sync job's second pass rewrites every imported row; it must survive
	// the same way.
	update := resourceToUpdateInput(created)
	update.AllowedUsers = nil
	update.AllowedGroups = nil
	if _, err := repo.Update(ctx, created.ID, update); err != nil {
		t.Fatalf("sync update with unset sharing lists: %v", err)
	}
}
