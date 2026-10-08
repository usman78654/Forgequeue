package main

import (
	"context"
	"github.com/usman78654/Forgequeue/internal/config"
	"github.com/usman78654/Forgequeue/internal/database"
	"github.com/usman78654/Forgequeue/internal/scheduler"
	"github.com/usman78654/Forgequeue/internal/tracing"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	shutdownTrace, err := tracing.Init(ctx, "forgequeue-scheduler")
	if err != nil {
		slog.Error("tracing", "error", err)
		os.Exit(1)
	}
	defer shutdownTrace(context.Background())
	db, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	go func() { _ = http.ListenAndServe(cfg.WorkerMetricsAddr, promhttp.Handler()) }()
	if err = scheduler.Run(ctx, db, cfg.WorkerID, cfg.SchedulerPollInterval); err != nil && ctx.Err() == nil {
		slog.Error("scheduler stopped", "error", err)
		os.Exit(1)
	}
}
