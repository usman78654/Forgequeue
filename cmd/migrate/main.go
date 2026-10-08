package main

import (
	"context"
	"github.com/usman78654/Forgequeue/internal/config"
	"github.com/usman78654/Forgequeue/internal/database"
	"github.com/usman78654/Forgequeue/internal/migrations"
	"log/slog"
	"os"
	"time"
)

func main() {
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
	slog.Info("database migrations complete")
}
