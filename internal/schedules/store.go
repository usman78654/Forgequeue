package schedules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

var ErrNotFound = errors.New("schedule not found")

type Schedule struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Type            string          `json:"type"`
	Queue           string          `json:"queue"`
	Payload         json.RawMessage `json:"payload"`
	IntervalSeconds int             `json:"interval_seconds"`
	Priority        int16           `json:"priority"`
	Enabled         bool            `json:"enabled"`
	NextRunAt       time.Time       `json:"next_run_at"`
}
type CreateRequest struct {
	Name            string          `json:"name"`
	Type            string          `json:"type"`
	Queue           string          `json:"queue,omitempty"`
	Payload         json.RawMessage `json:"payload"`
	IntervalSeconds int             `json:"interval_seconds"`
	Priority        int16           `json:"priority,omitempty"`
	MaxAttempts     int             `json:"max_attempts,omitempty"`
	TimeoutSeconds  int             `json:"timeout_seconds,omitempty"`
	StartAt         *time.Time      `json:"start_at,omitempty"`
	OwnerID         string          `json:"-"`
}
type Store struct{ DB *pgxpool.Pool }

func (s Store) Create(ctx context.Context, r CreateRequest) (Schedule, error) {
	if r.OwnerID != "" {
		var maximum, current int
		if err := s.DB.QueryRow(ctx, `SELECT max_schedules,(SELECT count(*) FROM recurring_schedules WHERE owner_id=$1) FROM tenant_quotas WHERE principal_id=$1`, r.OwnerID).Scan(&maximum, &current); err == nil && current >= maximum {
			return Schedule{}, fmt.Errorf("schedule quota exceeded: maximum %d", maximum)
		}
	}
	if r.Queue == "" {
		r.Queue = "jobs.default"
	}
	if r.MaxAttempts == 0 {
		r.MaxAttempts = 3
	}
	if r.TimeoutSeconds == 0 {
		r.TimeoutSeconds = 300
	}
	start := time.Now()
	if r.StartAt != nil {
		start = *r.StartAt
	}
	var out Schedule
	err := s.DB.QueryRow(ctx, `INSERT INTO recurring_schedules(name,job_type,queue,payload,interval_seconds,priority,max_attempts,timeout_seconds,next_run_at,owner_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,'')::uuid) RETURNING id,name,job_type,queue,payload,interval_seconds,priority,enabled,next_run_at`, r.Name, r.Type, r.Queue, r.Payload, r.IntervalSeconds, r.Priority, r.MaxAttempts, r.TimeoutSeconds, start, r.OwnerID).Scan(&out.ID, &out.Name, &out.Type, &out.Queue, &out.Payload, &out.IntervalSeconds, &out.Priority, &out.Enabled, &out.NextRunAt)
	return out, err
}
func (s Store) List(ctx context.Context) ([]Schedule, error) {
	rows, err := s.DB.Query(ctx, `SELECT id,name,job_type,queue,payload,interval_seconds,priority,enabled,next_run_at FROM recurring_schedules ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Schedule
	for rows.Next() {
		var x Schedule
		if err = rows.Scan(&x.ID, &x.Name, &x.Type, &x.Queue, &x.Payload, &x.IntervalSeconds, &x.Priority, &x.Enabled, &x.NextRunAt); err != nil {
			return nil, err
		}
		result = append(result, x)
	}
	return result, rows.Err()
}
func (s Store) SetEnabled(ctx context.Context, id string, enabled bool) error {
	tag, err := s.DB.Exec(ctx, `UPDATE recurring_schedules SET enabled=$2,next_run_at=CASE WHEN $2 THEN now() ELSE next_run_at END WHERE id=$1`, id, enabled)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
func (s Store) DispatchDue(ctx context.Context) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT id,job_type,queue,payload,priority,max_attempts,timeout_seconds,interval_seconds FROM recurring_schedules WHERE enabled AND next_run_at<=now() ORDER BY next_run_at FOR UPDATE SKIP LOCKED LIMIT 100`)
	if err != nil {
		return err
	}
	type due struct {
		id, typ, queue                 string
		payload                        []byte
		priority                       int16
		maxAttempts, timeout, interval int
	}
	var items []due
	for rows.Next() {
		var d due
		if err = rows.Scan(&d.id, &d.typ, &d.queue, &d.payload, &d.priority, &d.maxAttempts, &d.timeout, &d.interval); err != nil {
			rows.Close()
			return err
		}
		items = append(items, d)
	}
	rows.Close()
	for _, d := range items {
		var jobID string
		err = tx.QueryRow(ctx, `INSERT INTO jobs(type,queue,payload,status,priority,max_attempts,timeout_seconds,idempotency_key) VALUES($1,$2,$3,'queued',$4,$5,$6,$7) ON CONFLICT(idempotency_key) WHERE idempotency_key IS NOT NULL DO UPDATE SET updated_at=jobs.updated_at RETURNING id`, d.typ, d.queue, d.payload, d.priority, d.maxAttempts, d.timeout, d.id+":"+time.Now().UTC().Format("20060102150405")).Scan(&jobID)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO outbox(job_id,queue,priority) VALUES($1,$2,$3)`, jobID, d.queue, d.priority); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE recurring_schedules SET next_run_at=GREATEST(next_run_at,now())+make_interval(secs=>$2) WHERE id=$1`, d.id, d.interval); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) || errors.Is(err, pgx.ErrNoRows) }
