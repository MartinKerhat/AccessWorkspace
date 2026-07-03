package app

import (
	"context"
	"log"
	"strings"

	"access-workspace/backend/internal/auth"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// bootstrapAdmin creates the first administrator from the environment when the
// user table is empty. It is the deployment-friendly alternative to inserting a
// row by hand: set BOOTSTRAP_ADMIN_USERNAME / BOOTSTRAP_ADMIN_PASSWORD (via a
// Secret in production) and the first startup against an empty database creates
// an admin who can then sign in and configure everything else.
//
// It is idempotent: once any user exists it does nothing, so restarts and
// rolling deploys never recreate or overwrite the admin.
func bootstrapAdmin(ctx context.Context, pool *pgxpool.Pool, cfg Config) error {
	if cfg.BootstrapAdminUser == "" || cfg.BootstrapAdminPass == "" {
		return nil
	}

	var userCount int
	if err := pool.QueryRow(ctx, `select count(*) from app_users`).Scan(&userCount); err != nil {
		return err
	}
	if userCount > 0 {
		return nil
	}

	username := strings.ToLower(strings.TrimSpace(cfg.BootstrapAdminUser))
	displayName := strings.TrimSpace(cfg.BootstrapAdminName)
	if displayName == "" {
		displayName = username
	}
	email := strings.ToLower(strings.TrimSpace(cfg.BootstrapAdminEmail))

	passwordHash, err := auth.HashPassword(cfg.BootstrapAdminPass)
	if err != nil {
		return err
	}

	if _, err := pool.Exec(ctx, `
		insert into app_users (id, username, display_name, email, password_hash, groups, is_admin)
		values ($1, $2, $3, $4, $5, '{}', true)
	`, uuid.NewString(), username, displayName, email, passwordHash); err != nil {
		return err
	}

	log.Printf("bootstrapped initial admin user %q", username)
	return nil
}
