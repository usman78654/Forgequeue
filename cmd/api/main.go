package main

import (
	"context"
	"github.com/usman78654/Forgequeue/internal/artifacts"
	"github.com/usman78654/Forgequeue/internal/broker"
	"github.com/usman78654/Forgequeue/internal/config"
	"github.com/usman78654/Forgequeue/internal/database"
	"github.com/usman78654/Forgequeue/internal/httpapi"
	"github.com/usman78654/Forgequeue/internal/observability"
	"github.com/usman78654/Forgequeue/internal/tracing"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	shutdownTrace, e := tracing.Init(ctx, "forgequeue-api")
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
	go broker.RunOutbox(ctx, db, rabbit, cfg.OutboxPollInterval)
	go observability.RunCollector(ctx, db)
	artifactStore := &artifacts.Store{DB: db, Endpoint: cfg.S3Endpoint, PublicEndpoint: cfg.S3PublicEndpoint, AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey, Bucket: cfg.S3Bucket, MaxBytes: cfg.ArtifactMaxBytes, Retention: cfg.ArtifactRetention}
	if e = artifactStore.EnsureBucket(ctx); e != nil {
		slog.Warn("artifact storage unavailable", "error", e)
		artifactStore = nil
	} else {
		go artifactStore.RunCleanup(ctx)
	}
	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: httpapi.NewWithArtifacts(db, rabbit, artifactStore).ConfigureWebSessions(cfg.SessionDuration, cfg.SessionCookieSecure).Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(c)
	}()
	slog.Info("api listening", "address", cfg.HTTPAddr)
	if e = srv.ListenAndServe(); e != nil && !errorsIsServerClosed(e) {
		slog.Error("server", "error", e)
		os.Exit(1)
	}
}
func errorsIsServerClosed(e error) bool { return e == http.ErrServerClosed }
