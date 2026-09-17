package main

import (
	"context"
	"log"

	"github.com/MartinKerhat/AccessWorkspace/backend/internal/app"
	"github.com/MartinKerhat/AccessWorkspace/backend/internal/db"
	"github.com/MartinKerhat/AccessWorkspace/backend/internal/seed"
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
