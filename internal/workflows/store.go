package workflows

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/usman78654/Forgequeue/internal/pagination"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"slices"
)

var ErrNotFound = errors.New("workflow not found")

type Store struct{ DB *pgxpool.Pool }

func (s Store) List(ctx context.Context, filter ListFilter) (Page, error) {
	if filter.Limit < 1 || filter.Limit > 100 {
		filter.Limit = 50
	}
	cursor, err := pagination.Decode(filter.Cursor)
	if err != nil {
		return Page{}, err
	}
	var cursorAt, cursorID any
	if !cursor.CreatedAt.IsZero() {
		cursorAt = cursor.CreatedAt
		cursorID = cursor.ID
	}
	rows, err := s.DB.Query(ctx, `SELECT id,name,status,fail_fast,created_at,updated_at,owner_id FROM workflows WHERE (NULLIF($1,'') IS NULL OR status::text=$1) AND (NULLIF($2,'') IS NULL OR name ILIKE '%'||$2||'%') AND ($3::timestamptz IS NULL OR created_at >= $3) AND ($4::timestamptz IS NULL OR created_at <= $4) AND ($5::timestamptz IS NULL OR (created_at,id)<($5,$6::uuid)) AND (NULLIF($7,'') IS NULL OR owner_id=$7::uuid) ORDER BY created_at DESC,id DESC LIMIT $8`, filter.Status, filter.Name, filter.CreatedAfter, filter.CreatedBefore, cursorAt, cursorID, filter.OwnerID, filter.Limit+1)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	var items []Workflow
	for rows.Next() {
		var w Workflow
		if err = rows.Scan(&w.ID, &w.Name, &w.Status, &w.FailFast, &w.CreatedAt, &w.UpdatedAt, &w.OwnerID); err != nil {
			return Page{}, err
		}
		items = append(items, w)
	}
	if err = rows.Err(); err != nil {
		return Page{}, err
	}
	page := Page{Items: items}
	if len(items) > filter.Limit {
		last := items[filter.Limit-1]
		page.Items = items[:filter.Limit]
		page.NextCursor = pagination.Encode(last.CreatedAt, last.ID)
	}
	return page, nil
}

func (s Store) Submit(ctx context.Context, r SubmitRequest) (Workflow, error) {
	if err := Validate(r); err != nil {
		return Workflow{}, err
	}
	failFast := true
	if r.FailFast != nil {
		failFast = *r.FailFast
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Workflow{}, err
	}
	defer tx.Rollback(ctx)
	var w Workflow
	err = tx.QueryRow(ctx, `INSERT INTO workflows(name,fail_fast,owner_id,traceparent) VALUES($1,$2,NULLIF($3,'')::uuid,NULLIF($4,'')) RETURNING id,name,status,fail_fast,created_at,updated_at,owner_id`, r.Name, failFast, r.OwnerID, r.Traceparent).Scan(&w.ID, &w.Name, &w.Status, &w.FailFast, &w.CreatedAt, &w.UpdatedAt, &w.OwnerID)
	if err != nil {
		return Workflow{}, err
	}
	for _, step := range r.Steps {
		if step.DependsOn == nil {
			step.DependsOn = []string{}
		}
		if step.Queue == "" {
			step.Queue = "jobs.default"
		}
		if step.MaxAttempts == 0 {
			step.MaxAttempts = 3
		}
		if step.TimeoutSeconds == 0 {
			step.TimeoutSeconds = 300
		}
		status := "pending"
		if step.FanOutFrom != "" && !slices.Contains(step.DependsOn, step.FanOutFrom) {
			step.DependsOn = append(step.DependsOn, step.FanOutFrom)
		}
		if len(step.DependsOn) == 0 && step.FanOutFrom == "" {
			status = "queued"
		}
		var jobID *string
		if status == "queued" {
			var id string
			err = tx.QueryRow(ctx, `INSERT INTO jobs(type,queue,payload,status,priority,max_attempts,timeout_seconds,workflow_id,workflow_step_key,owner_id) VALUES($1,$2,$3,'queued',$4,$5,$6,$7,$8,$9) RETURNING id`, step.Type, step.Queue, step.Payload, step.Priority, step.MaxAttempts, step.TimeoutSeconds, w.ID, step.Key, w.OwnerID).Scan(&id)
			if err != nil {
				return Workflow{}, err
			}
			jobID = &id
			if _, err = tx.Exec(ctx, `INSERT INTO outbox(job_id,queue,priority) VALUES($1,$2,$3)`, id, step.Queue, step.Priority); err != nil {
				return Workflow{}, err
			}
		}
		compensation, _ := json.Marshal(step.Compensation)
		if step.Compensation == nil {
			compensation = nil
		}
		_, err = tx.Exec(ctx, `INSERT INTO workflow_steps(workflow_id,step_key,job_type,queue,payload,dependencies,status,priority,max_attempts,timeout_seconds,job_id,condition,continue_on_error,fan_out_from,compensation) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NULLIF($12,''),$13,NULLIF($14,''),$15)`, w.ID, step.Key, step.Type, step.Queue, step.Payload, step.DependsOn, status, step.Priority, step.MaxAttempts, step.TimeoutSeconds, jobID, step.Condition, step.ContinueOnError, step.FanOutFrom, compensation)
		if err != nil {
			return Workflow{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return Workflow{}, err
	}
	return s.Get(ctx, w.ID)
}
func (s Store) Get(ctx context.Context, id string) (Workflow, error) {
	var w Workflow
	err := s.DB.QueryRow(ctx, `SELECT id,name,status,fail_fast,created_at,updated_at,owner_id FROM workflows WHERE id=$1`, id).Scan(&w.ID, &w.Name, &w.Status, &w.FailFast, &w.CreatedAt, &w.UpdatedAt, &w.OwnerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Workflow{}, ErrNotFound
	}
	if err != nil {
		return Workflow{}, err
	}
	rows, err := s.DB.Query(ctx, `SELECT step_key,job_type,status,dependencies,job_id,COALESCE(condition,''),continue_on_error,output,COALESCE(fan_out_from,''),COALESCE(parent_step_key,''),is_compensation FROM workflow_steps WHERE workflow_id=$1 ORDER BY created_at,step_key`, id)
	if err != nil {
		return Workflow{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var step Step
		if err = rows.Scan(&step.Key, &step.Type, &step.Status, &step.DependsOn, &step.JobID, &step.Condition, &step.ContinueOnError, &step.Output, &step.FanOutFrom, &step.ParentStepKey, &step.IsCompensation); err != nil {
			return Workflow{}, err
		}
		w.Steps = append(w.Steps, step)
	}
	return w, rows.Err()
}
func (s Store) Cancel(ctx context.Context, id, reason string) error {
	if reason == "" {
		reason = "workflow cancelled by user"
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE workflows SET status='cancelled',updated_at=now(),finished_at=now() WHERE id=$1 AND status='running'`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	_, err = tx.Exec(ctx, `UPDATE workflow_steps SET status='cancelled',updated_at=now() WHERE workflow_id=$1 AND status IN ('pending','queued')`, id)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE jobs SET status='cancelled',cancellation_requested_at=now(),cancellation_reason=$2,updated_at=now(),finished_at=now() WHERE workflow_id=$1 AND status IN ('scheduled','queued','retrying')`, id, reason); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE jobs SET status='cancelling',cancellation_requested_at=now(),cancellation_reason=$2,updated_at=now() WHERE workflow_id=$1 AND status='running'`, id, reason); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
