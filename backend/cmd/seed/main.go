package main

import (
	"context"
	"log"

	"access-workspace/backend/internal/app"
	"access-workspace/backend/internal/db"
	"access-workspace/backend/internal/seed"
)

func main() {
	cfg := app.ConfigFromEnv()

	pool, err := db.Open(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer pool.Close()

	if err := db.RunMigrations(context.Background(), pool); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	if err := seed.Run(context.Background(), pool); err != nil {
		log.Fatalf("seed: %v", err)
	}

	log.Println("seed completed")
}
