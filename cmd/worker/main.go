package main

import (
	"context"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/usman78654/Forgequeue/internal/broker"
	"github.com/usman78654/Forgequeue/internal/config"
	"github.com/usman78654/Forgequeue/internal/database"
	"github.com/usman78654/Forgequeue/internal/jobs"
	"github.com/usman78654/Forgequeue/internal/tracing"
	workerpkg "github.com/usman78654/Forgequeue/internal/worker"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	shutdownTrace, e := tracing.Init(ctx, "forgequeue-worker")
	if e != nil {
		slog.Error("tracing", "error", e)
		os.Exit(1)
	}
	defer shutdownTrace(context.Background())
	db, e := database.Open(ctx, cfg.DatabaseURL)
	if e != nil {
		slog.Error("database", "error", e)
		os.Exit(1)
	}
	defer db.Close()
	rabbit, e := broker.Open(cfg.RabbitMQURL)
	if e != nil {
		slog.Error("rabbitmq", "error", e)
		os.Exit(1)
	}
	defer rabbit.Close()
	metricsMux := http.NewServeMux()
	var draining atomic.Bool
	metricsMux.Handle("GET /metrics", promhttp.Handler())
	metricsMux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if draining.Load() {
			http.Error(w, "worker draining", http.StatusServiceUnavailable)
			return
		}
		if !rabbit.Healthy() {
			http.Error(w, "rabbitmq unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	metrics := &http.Server{Addr: cfg.WorkerMetricsAddr, Handler: metricsMux}
	go metrics.ListenAndServe()
	go func() { <-ctx.Done(); _ = metrics.Shutdown(context.Background()) }()
	w := workerpkg.Worker{Store: jobs.Store{DB: db}, Rabbit: rabbit, ID: cfg.WorkerID, Queue: cfg.QueueName, Queues: cfg.QueueNames, Capabilities: cfg.WorkerCapabilities, Concurrency: cfg.WorkerConcurrency, Lease: cfg.LeaseDuration, DrainTimeout: cfg.DrainTimeout, AllowCommand: cfg.AllowCommandHandler, Draining: &draining}
	slog.Info("worker started", "id", cfg.WorkerID, "concurrency", cfg.WorkerConcurrency)
	if e = w.Run(ctx); e != nil && ctx.Err() == nil {
		slog.Error("worker stopped", "error", e)
		os.Exit(1)
	}
}
