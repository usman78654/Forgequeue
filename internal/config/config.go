package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr, WorkerMetricsAddr, DatabaseURL, RabbitMQURL, QueueName, WorkerID string
	QueueNames, WorkerCapabilities                                             []string
	WorkerConcurrency                                                          int
	LeaseDuration, DrainTimeout, OutboxPollInterval, SchedulerPollInterval     time.Duration
	AllowCommandHandler                                                        bool
	S3Endpoint, S3AccessKey, S3SecretKey, S3Bucket, S3PublicEndpoint           string
	ArtifactMaxBytes                                                           int64
	ArtifactRetention                                                          time.Duration
}

func Load() Config {
	host, _ := os.Hostname()
	return Config{
		HTTPAddr: env("HTTP_ADDR", ":8080"), DatabaseURL: env("DATABASE_URL", "postgres://queue:queue@localhost:5432/queue?sslmode=disable"),
		WorkerMetricsAddr: env("WORKER_METRICS_ADDR", ":9091"),
		RabbitMQURL:       env("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"), QueueName: env("QUEUE_NAME", "jobs.default"),
		QueueNames: splitEnv("QUEUE_NAMES", env("QUEUE_NAME", "jobs.default")), WorkerCapabilities: splitEnv("WORKER_CAPABILITIES", "echo,sleep"),
		WorkerID: env("WORKER_ID", host), WorkerConcurrency: envInt("WORKER_CONCURRENCY", 4),
		LeaseDuration: envDuration("LEASE_DURATION", 30*time.Second), OutboxPollInterval: envDuration("OUTBOX_POLL_INTERVAL", 500*time.Millisecond),
		SchedulerPollInterval: envDuration("SCHEDULER_POLL_INTERVAL", time.Second),
		DrainTimeout: envDuration("WORKER_DRAIN_TIMEOUT", 45*time.Second), AllowCommandHandler: envBool("ALLOW_COMMAND_HANDLER", false),
		S3Endpoint: env("S3_ENDPOINT", "localhost:9000"), S3PublicEndpoint: env("S3_PUBLIC_ENDPOINT", "localhost:9000"),
		S3AccessKey: env("S3_ACCESS_KEY", "minioadmin"), S3SecretKey: env("S3_SECRET_KEY", "minioadmin"), S3Bucket: env("S3_BUCKET", "forgequeue"),
		ArtifactMaxBytes: int64(envInt("ARTIFACT_MAX_BYTES", 50<<20)), ArtifactRetention: envDuration("ARTIFACT_RETENTION", 7*24*time.Hour),
	}
}
func envBool(k string, d bool) bool {
	v := os.Getenv(k); if v == "" { return d }
	b, e := strconv.ParseBool(v); if e != nil { return d }; return b
}
func splitEnv(k, d string) []string {
	v := env(k, d)
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func envInt(k string, d int) int {
	v, e := strconv.Atoi(os.Getenv(k))
	if e == nil && v > 0 {
		return v
	}
	return d
}
func envDuration(k string, d time.Duration) time.Duration {
	v, e := time.ParseDuration(os.Getenv(k))
	if e == nil && v > 0 {
		return v
	}
	return d
}
