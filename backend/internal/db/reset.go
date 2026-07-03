package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func ResetPublicSchema(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		drop schema if exists public cascade;
		create schema public;
		grant all on schema public to postgres;
		grant all on schema public to public;
	`)
	return err
}
