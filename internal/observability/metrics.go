package observability

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"log/slog"
	"time"
)

var jobStates = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "forgequeue_jobs", Help: "Current number of jobs by state."}, []string{"status"})
var activeWorkers = prometheus.NewGauge(prometheus.GaugeOpts{Name: "forgequeue_active_workers", Help: "Workers with a heartbeat in the last 30 seconds."})
var outboxPending = prometheus.NewGauge(prometheus.GaugeOpts{Name: "forgequeue_outbox_pending", Help: "Outbox records waiting to publish."})
var queueDepth = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "forgequeue_queue_depth", Help: "Runnable jobs by queue."}, []string{"queue"})
var expiredLeases = prometheus.NewGauge(prometheus.GaugeOpts{Name: "forgequeue_expired_leases", Help: "Running jobs whose lease has expired."})
var oldestOutbox = prometheus.NewGauge(prometheus.GaugeOpts{Name: "forgequeue_outbox_oldest_seconds", Help: "Age of the oldest unpublished outbox message."})

func init() {
	prometheus.MustRegister(jobStates, activeWorkers, outboxPending, queueDepth, expiredLeases, oldestOutbox)
}
func RunCollector(ctx context.Context, db *pgxpool.Pool) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		collect(ctx, db)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func collect(ctx context.Context, db *pgxpool.Pool) {
	rows, err := db.Query(ctx, `SELECT status::text,count(*) FROM jobs GROUP BY status`)
	if err != nil {
		slog.Warn("metrics collection failed", "error", err)
		return
	}
	jobStates.Reset()
	for rows.Next() {
		var status string
		var count int64
		if rows.Scan(&status, &count) == nil {
			jobStates.WithLabelValues(status).Set(float64(count))
		}
	}
	rows.Close()
	queueDepth.Reset()
	rows, err = db.Query(ctx, `SELECT queue,count(*) FROM jobs WHERE status IN ('queued','retrying') AND scheduled_at<=now() GROUP BY queue`)
	if err == nil {
		for rows.Next() {
			var queue string
			var count int64
			if rows.Scan(&queue, &count) == nil {
				queueDepth.WithLabelValues(queue).Set(float64(count))
			}
		}
		rows.Close()
	}
	var workers, pending, expired int64
	if db.QueryRow(ctx, `SELECT count(*) FROM workers WHERE state='running' AND last_heartbeat_at>now()-interval '30 seconds'`).Scan(&workers) == nil {
		activeWorkers.Set(float64(workers))
	}
	if db.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE published_at IS NULL`).Scan(&pending) == nil {
		outboxPending.Set(float64(pending))
	}
	if db.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE status IN ('running','cancelling') AND lease_expires_at<now()`).Scan(&expired) == nil {
		expiredLeases.Set(float64(expired))
	}
	var age float64
	if db.QueryRow(ctx, `SELECT COALESCE(EXTRACT(EPOCH FROM now()-min(created_at)),0) FROM outbox WHERE published_at IS NULL`).Scan(&age) == nil {
		oldestOutbox.Set(age)
	}
}
