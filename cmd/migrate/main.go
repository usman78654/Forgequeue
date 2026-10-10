package main

import (
	"context"
	"flag"
	"github.com/usman78654/Forgequeue/internal/config"
	"github.com/usman78654/Forgequeue/internal/database"
	"github.com/usman78654/Forgequeue/internal/migrations"
	"log/slog"
	"os"
	"time"
)

func main() {
	seedDevelopment := flag.Bool("seed-development", false, "install deterministic local-only API keys")
	flag.Parse()
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	db, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	if err = migrations.Run(ctx, db); err != nil {
		slog.Error("migration failed", "error", err)
		os.Exit(1)
	}
	if *seedDevelopment {
		if err = migrations.SeedDevelopment(ctx, db); err != nil {
			slog.Error("development seed failed", "error", err)
			os.Exit(1)
		}
		slog.Warn("local development credentials installed; do not use this mode in production")
	}
	slog.Info("database migrations complete")
}
