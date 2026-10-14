package scheduler

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/usman78654/Forgequeue/internal/jobs"
	"github.com/usman78654/Forgequeue/internal/schedules"
	"log/slog"
	"time"
)

const advisoryLockID int64 = 0x464F524745515545

var leader = prometheus.NewGauge(prometheus.GaugeOpts{Name: "forgequeue_scheduler_leader", Help: "1 when this process holds scheduler leadership."})
var transitions = prometheus.NewCounter(prometheus.CounterOpts{Name: "forgequeue_scheduler_leadership_transitions_total", Help: "Scheduler leadership acquisitions."})

func init() { prometheus.MustRegister(leader, transitions) }
func Run(ctx context.Context, db *pgxpool.Pool, id string, every time.Duration) error {
	for ctx.Err() == nil {
		conn, err := db.Acquire(ctx)
		if err != nil {
			return err
		}
		var acquired bool
		err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, advisoryLockID).Scan(&acquired)
		if err != nil {
			conn.Release()
			return err
		}
		if !acquired {
			conn.Release()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(every):
				continue
			}
		}
		leader.Set(1)
		transitions.Inc()
		slog.Info("scheduler leadership acquired", "holder", id)
		_, _ = conn.Exec(ctx, `INSERT INTO scheduler_leases(name,holder_id) VALUES('primary',$1) ON CONFLICT(name) DO UPDATE SET holder_id=EXCLUDED.holder_id,acquired_at=now(),heartbeat_at=now()`, id)
		ticker := time.NewTicker(every)
		jobStore := jobs.Store{DB: db}
		scheduleStore := schedules.Store{DB: db}
		lost := false
		for !lost {
			select {
			case <-ctx.Done():
				lost = true
			case <-ticker.C:
				if _, err = conn.Exec(ctx, `UPDATE scheduler_leases SET heartbeat_at=now() WHERE name='primary' AND holder_id=$1`, id); err != nil {
					lost = true
					break
				}
				if err = jobStore.PromoteScheduled(ctx); err != nil {
					slog.Warn("promote scheduled jobs", "error", err)
				}
				if err = jobStore.RecoverExpired(ctx); err != nil {
					slog.Warn("recover leases", "error", err)
				}
				if err = scheduleStore.DispatchDue(ctx); err != nil {
					slog.Warn("dispatch schedules", "error", err)
				}
			}
		}
		ticker.Stop()
		leader.Set(0)
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, advisoryLockID)
		conn.Release()
	}
	return ctx.Err()
}
