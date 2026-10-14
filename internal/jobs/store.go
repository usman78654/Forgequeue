package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/usman78654/Forgequeue/internal/pagination"
)

var ErrNotFound = errors.New("job not found")
var ErrLeaseLost = errors.New("job lease is no longer owned by this worker")
var ErrCancellationRequested = errors.New("job cancellation was requested")
var ErrQuotaExceeded = errors.New("tenant quota exceeded")
var ErrRoutingBlocked = errors.New("job is blocked by queue or capability routing")

type Store struct{ DB *pgxpool.Pool }

func (s Store) Submit(ctx context.Context, r SubmitRequest) (Job, bool, error) {
	if r.OwnerID != "" {
		var perMinute, maxConcurrent, maxPayload int
		err := s.DB.QueryRow(ctx, `SELECT submissions_per_minute,max_concurrent_jobs,max_payload_bytes FROM tenant_quotas WHERE principal_id=$1`, r.OwnerID).Scan(&perMinute, &maxConcurrent, &maxPayload)
		if err == nil {
			var recent, active int
			_ = s.DB.QueryRow(ctx, `SELECT count(*) FILTER(WHERE created_at>now()-interval '1 minute'),count(*) FILTER(WHERE status IN ('scheduled','queued','running','retrying')) FROM jobs WHERE owner_id=$1`, r.OwnerID).Scan(&recent, &active)
			if len(r.Payload) > maxPayload {
				return Job{}, false, fmt.Errorf("%w: payload exceeds %d bytes", ErrQuotaExceeded, maxPayload)
			}
			if recent >= perMinute {
				return Job{}, false, fmt.Errorf("%w: submission rate exceeds %d/minute", ErrQuotaExceeded, perMinute)
			}
			if active >= maxConcurrent {
				return Job{}, false, fmt.Errorf("%w: concurrent jobs exceed %d", ErrQuotaExceeded, maxConcurrent)
			}
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
	if r.RetryPolicy == "" {
		r.RetryPolicy = "exponential"
	}
	if r.RetryInitialSeconds == 0 {
		r.RetryInitialSeconds = 2
	}
	if r.RetryMaxSeconds == 0 {
		r.RetryMaxSeconds = 300
	}
	retryJitter := true
	if r.RetryJitter != nil {
		retryJitter = *r.RetryJitter
	}
	if r.NonRetryableErrors == nil {
		r.NonRetryableErrors = []string{}
	}
	if r.RequiredCapabilities == nil {
		r.RequiredCapabilities = []string{}
	}
	when := time.Now().UTC()
	if r.ScheduledAt != nil {
		when = r.ScheduledAt.UTC()
	}
	status := "queued"
	if when.After(time.Now()) {
		status = "scheduled"
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Job{}, false, err
	}
	defer tx.Rollback(ctx)
	var j Job
	err = tx.QueryRow(ctx, `INSERT INTO jobs(type,queue,payload,status,priority,idempotency_key,max_attempts,timeout_seconds,scheduled_at,owner_id,retry_policy,retry_initial_seconds,retry_max_seconds,retry_jitter,non_retryable_errors,required_capabilities,traceparent)
		VALUES($1,$2,$3,$4,$5,NULLIF($6,''),$7,$8,$9,NULLIF($10,'')::uuid,$11,$12,$13,$14,$15,$16,NULLIF($17,''))
		ON CONFLICT (idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING
		RETURNING id,type,queue,payload,status,priority,attempt,max_attempts,timeout_seconds,scheduled_at,last_error,result,created_at,updated_at,cancellation_requested_at,cancellation_reason,owner_id,retry_policy,retry_initial_seconds,retry_max_seconds,retry_jitter,non_retryable_errors`,
		r.Type, r.Queue, r.Payload, status, r.Priority, r.IdempotencyKey, r.MaxAttempts, r.TimeoutSeconds, when, r.OwnerID, r.RetryPolicy, r.RetryInitialSeconds, r.RetryMaxSeconds, retryJitter, r.NonRetryableErrors, r.RequiredCapabilities, r.Traceparent).Scan(&j.ID, &j.Type, &j.Queue, &j.Payload, &j.Status, &j.Priority, &j.Attempt, &j.MaxAttempts, &j.TimeoutSeconds, &j.ScheduledAt, &j.LastError, &j.Result, &j.CreatedAt, &j.UpdatedAt, &j.CancellationRequestedAt, &j.CancellationReason, &j.OwnerID, &j.RetryPolicy, &j.RetryInitialSeconds, &j.RetryMaxSeconds, &j.RetryJitter, &j.NonRetryableErrors)
	created := true
	if errors.Is(err, pgx.ErrNoRows) && r.IdempotencyKey != "" {
		created = false
		j, err = getWith(ctx, tx, "idempotency_key", r.IdempotencyKey)
	}
	if err != nil {
		return Job{}, false, err
	}
	if created && status == "queued" {
		_, err = tx.Exec(ctx, `INSERT INTO outbox(job_id,queue,priority,traceparent) VALUES($1,$2,$3,NULLIF($4,''))`, j.ID, j.Queue, j.Priority, r.Traceparent)
	}
	if err != nil {
		return Job{}, false, err
	}
	return j, created, tx.Commit(ctx)
}

func (s Store) Get(ctx context.Context, id string) (Job, error) { return getWith(ctx, s.DB, "id", id) }

func (s Store) List(ctx context.Context, filter ListFilter) (Page, error) {
	if filter.Limit < 1 || filter.Limit > 100 {
		filter.Limit = 50
	}
	cursor, err := pagination.Decode(filter.Cursor)
	if err != nil {
		return Page{}, err
	}
	var cursorAt any
	var cursorID any
	if !cursor.CreatedAt.IsZero() {
		cursorAt = cursor.CreatedAt
		cursorID = cursor.ID
	}
	rows, err := s.DB.Query(ctx, `SELECT id,type,queue,payload,status,priority,attempt,max_attempts,timeout_seconds,scheduled_at,last_error,result,created_at,updated_at,cancellation_requested_at,cancellation_reason,owner_id,retry_policy,retry_initial_seconds,retry_max_seconds,retry_jitter,non_retryable_errors FROM jobs
		WHERE (NULLIF($1,'') IS NULL OR status::text=$1) AND (NULLIF($2,'') IS NULL OR queue=$2) AND (NULLIF($3,'') IS NULL OR type=$3)
		AND ($4::timestamptz IS NULL OR created_at >= $4) AND ($5::timestamptz IS NULL OR created_at <= $5)
		AND ($6::timestamptz IS NULL OR (created_at,id) < ($6,$7::uuid)) AND (NULLIF($8,'') IS NULL OR owner_id=$8::uuid) ORDER BY created_at DESC,id DESC LIMIT $9`, filter.Status, filter.Queue, filter.Type, filter.CreatedAfter, filter.CreatedBefore, cursorAt, cursorID, filter.OwnerID, filter.Limit+1)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	var result []Job
	for rows.Next() {
		var j Job
		if err = rows.Scan(&j.ID, &j.Type, &j.Queue, &j.Payload, &j.Status, &j.Priority, &j.Attempt, &j.MaxAttempts, &j.TimeoutSeconds, &j.ScheduledAt, &j.LastError, &j.Result, &j.CreatedAt, &j.UpdatedAt, &j.CancellationRequestedAt, &j.CancellationReason, &j.OwnerID, &j.RetryPolicy, &j.RetryInitialSeconds, &j.RetryMaxSeconds, &j.RetryJitter, &j.NonRetryableErrors); err != nil {
			return Page{}, err
		}
		result = append(result, j)
	}
	if err = rows.Err(); err != nil {
		return Page{}, err
	}
	page := Page{Items: result}
	if len(result) > filter.Limit {
		last := result[filter.Limit-1]
		page.Items = result[:filter.Limit]
		page.NextCursor = pagination.Encode(last.CreatedAt, last.ID)
	}
	return page, nil
}

func (s Store) Attempts(ctx context.Context, id string) ([]Attempt, error) {
	rows, err := s.DB.Query(ctx, `SELECT attempt,worker_id,started_at,finished_at,error FROM job_attempts WHERE job_id=$1 ORDER BY attempt`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Attempt
	for rows.Next() {
		var a Attempt
		if err = rows.Scan(&a.Attempt, &a.WorkerID, &a.StartedAt, &a.FinishedAt, &a.Error); err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

func (s Store) RequeueDead(ctx context.Context, id string) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var queue string
	var priority int16
	err = tx.QueryRow(ctx, `UPDATE jobs SET status='queued',attempt=0,last_error=NULL,finished_at=NULL,scheduled_at=now(),updated_at=now() WHERE id=$1 AND status='dead' RETURNING queue,priority`, id).Scan(&queue, &priority)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO outbox(job_id,queue,priority) VALUES($1,$2,$3)`, id, queue, priority); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func getWith(ctx context.Context, q rowQuerier, col, value string) (Job, error) {
	var j Job
	query := fmt.Sprintf(`SELECT id,type,queue,payload,status,priority,attempt,max_attempts,timeout_seconds,scheduled_at,last_error,result,created_at,updated_at,cancellation_requested_at,cancellation_reason,owner_id,retry_policy,retry_initial_seconds,retry_max_seconds,retry_jitter,non_retryable_errors FROM jobs WHERE %s=$1`, col)
	err := q.QueryRow(ctx, query, value).Scan(&j.ID, &j.Type, &j.Queue, &j.Payload, &j.Status, &j.Priority, &j.Attempt, &j.MaxAttempts, &j.TimeoutSeconds, &j.ScheduledAt, &j.LastError, &j.Result, &j.CreatedAt, &j.UpdatedAt, &j.CancellationRequestedAt, &j.CancellationReason, &j.OwnerID, &j.RetryPolicy, &j.RetryInitialSeconds, &j.RetryMaxSeconds, &j.RetryJitter, &j.NonRetryableErrors)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	return j, err
}

func (s Store) Cancel(ctx context.Context, id, reason string) error {
	if reason == "" {
		reason = "cancelled by user"
	}
	var status string
	var attempt int
	err := s.DB.QueryRow(ctx, `UPDATE jobs SET status=CASE WHEN status='running' THEN 'cancelling'::job_status ELSE 'cancelled'::job_status END,cancellation_requested_at=now(),cancellation_reason=$2,updated_at=now(),finished_at=CASE WHEN status='running' THEN NULL ELSE now() END WHERE id=$1 AND status IN ('scheduled','queued','retrying','running') RETURNING status,attempt`, id, reason).Scan(&status, &attempt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	message := "job cancellation requested: " + reason
	if status == "cancelled" {
		message = "job cancelled: " + reason
	}
	_, err = s.AppendEvent(ctx, id, EventInput{Attempt: attempt, Type: "state", Level: "warn", Message: message})
	return err
}

func (s Store) PromoteScheduled(ctx context.Context) error {
	_, err := s.DB.Exec(ctx, `WITH promoted AS (UPDATE jobs SET status='queued',updated_at=now() WHERE id IN
		(SELECT id FROM jobs WHERE status IN ('scheduled','retrying') AND scheduled_at<=now() FOR UPDATE SKIP LOCKED LIMIT 100) RETURNING id,queue,priority)
		INSERT INTO outbox(job_id,queue,priority) SELECT id,queue,priority FROM promoted`)
	return err
}

func (s Store) Claim(ctx context.Context, id, worker string, lease time.Duration, capabilities ...string) (Job, error) {
	if capabilities == nil {
		capabilities = []string{}
	}
	tx, beginErr := s.DB.Begin(ctx)
	if beginErr != nil {
		return Job{}, beginErr
	}
	defer tx.Rollback(ctx)
	var j Job
	err := tx.QueryRow(ctx, `UPDATE jobs SET status='running',attempt=attempt+1,lease_token=lease_token+1,worker_id=$2,lease_expires_at=now()+$3::interval,updated_at=now()
		WHERE id=$1 AND status='queued' AND required_capabilities<@$4::text[] AND NOT EXISTS(SELECT 1 FROM queue_controls q WHERE q.queue=jobs.queue AND q.paused) AND NOT EXISTS(SELECT 1 FROM queue_controls q WHERE q.queue=jobs.queue AND q.max_concurrency IS NOT NULL AND (SELECT count(*) FROM jobs active WHERE active.queue=jobs.queue AND active.status='running')>=q.max_concurrency) RETURNING id,type,queue,payload,status,priority,attempt,lease_token,max_attempts,timeout_seconds,scheduled_at,last_error,result,created_at,updated_at,cancellation_requested_at,cancellation_reason,owner_id,retry_policy,retry_initial_seconds,retry_max_seconds,retry_jitter,non_retryable_errors`, id, worker, lease.String(), capabilities).Scan(&j.ID, &j.Type, &j.Queue, &j.Payload, &j.Status, &j.Priority, &j.Attempt, &j.LeaseToken, &j.MaxAttempts, &j.TimeoutSeconds, &j.ScheduledAt, &j.LastError, &j.Result, &j.CreatedAt, &j.UpdatedAt, &j.CancellationRequestedAt, &j.CancellationReason, &j.OwnerID, &j.RetryPolicy, &j.RetryInitialSeconds, &j.RetryMaxSeconds, &j.RetryJitter, &j.NonRetryableErrors)
	if errors.Is(err, pgx.ErrNoRows) {
		var queued bool
		if check := tx.QueryRow(ctx, `SELECT status='queued' FROM jobs WHERE id=$1`, id).Scan(&queued); check == nil && queued {
			return Job{}, ErrRoutingBlocked
		}
		return Job{}, ErrNotFound
	}
	if err != nil {
		return Job{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO job_attempts(job_id,attempt,worker_id,lease_token) VALUES($1,$2,$3,$4)`, id, j.Attempt, worker, j.LeaseToken); err != nil {
		return Job{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workflow_steps SET status='running',updated_at=now() WHERE job_id=$1`, id); err != nil {
		return Job{}, err
	}
	return j, tx.Commit(ctx)
}
func (s Store) Succeed(ctx context.Context, job Job, result []byte) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE jobs SET status='succeeded',result=$3,lease_expires_at=NULL,updated_at=now(),finished_at=now() WHERE id=$1 AND lease_token=$2 AND status='running' AND cancellation_requested_at IS NULL`, job.ID, job.LeaseToken, result)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrLeaseLost
	}
	if _, err = tx.Exec(ctx, `UPDATE job_attempts SET finished_at=now() WHERE job_id=$1 AND lease_token=$2`, job.ID, job.LeaseToken); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE workflow_steps SET status='succeeded',output=$2,updated_at=now() WHERE job_id=$1`, job.ID, result); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE workflow_steps parent SET status='succeeded',output=(SELECT jsonb_agg(child.output ORDER BY child.step_key) FROM workflow_steps child WHERE child.workflow_id=parent.workflow_id AND child.parent_step_key=parent.step_key),updated_at=now() WHERE (parent.workflow_id,parent.step_key)=(SELECT workflow_id,parent_step_key FROM workflow_steps WHERE job_id=$1) AND parent.parent_step_key IS NULL AND NOT EXISTS(SELECT 1 FROM workflow_steps child WHERE child.workflow_id=parent.workflow_id AND child.parent_step_key=parent.step_key AND child.status<>'succeeded')`, job.ID); err != nil {
		return err
	}
	var compensation bool
	if err = tx.QueryRow(ctx, `SELECT COALESCE((SELECT is_compensation FROM workflow_steps WHERE job_id=$1),false)`, job.ID).Scan(&compensation); err != nil {
		return err
	}
	if compensation {
		_, err = tx.Exec(ctx, `UPDATE workflows w SET status='compensated',updated_at=now(),finished_at=now() WHERE w.id=(SELECT workflow_id FROM jobs WHERE id=$1) AND w.status='compensating' AND NOT EXISTS(SELECT 1 FROM workflow_steps s WHERE s.workflow_id=w.id AND s.is_compensation AND s.status<>'succeeded')`, job.ID)
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	rows, err := tx.Query(ctx, `SELECT s.workflow_id,s.step_key,s.job_type,s.queue,s.payload,s.priority,s.max_attempts,s.timeout_seconds,COALESCE(s.condition,'success()'),COALESCE(s.fan_out_from,'')
		FROM workflow_steps s WHERE s.workflow_id=(SELECT workflow_id FROM jobs WHERE id=$1) AND s.status='pending'
		AND NOT EXISTS (SELECT 1 FROM unnest(s.dependencies) d LEFT JOIN workflow_steps dep ON dep.workflow_id=s.workflow_id AND dep.step_key=d WHERE dep.status NOT IN ('succeeded','failed','skipped'))
		FOR UPDATE OF s`, job.ID)
	if err != nil {
		return err
	}
	type readyStep struct {
		workflowID, key, jobType, queue string
		payload                         []byte
		priority                        int16
		maxAttempts, timeout            int
		condition                       string
		fanOutFrom                      string
	}
	var ready []readyStep
	for rows.Next() {
		var step readyStep
		if err = rows.Scan(&step.workflowID, &step.key, &step.jobType, &step.queue, &step.payload, &step.priority, &step.maxAttempts, &step.timeout, &step.condition, &step.fanOutFrom); err != nil {
			rows.Close()
			return err
		}
		ready = append(ready, step)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, step := range ready {
		run, evalErr := evaluateCondition(ctx, tx, step.workflowID, step.key, step.condition)
		if evalErr != nil {
			return evalErr
		}
		if !run {
			if _, err = tx.Exec(ctx, `UPDATE workflow_steps SET status='skipped',updated_at=now() WHERE workflow_id=$1 AND step_key=$2`, step.workflowID, step.key); err != nil {
				return err
			}
			continue
		}
		if step.fanOutFrom != "" {
			if err = expandFanOut(ctx, tx, step.workflowID, step.key, step.jobType, step.queue, step.payload, step.priority, step.maxAttempts, step.timeout, step.fanOutFrom); err != nil {
				return err
			}
			continue
		}
		step.payload, err = resolvePayload(ctx, tx, step.workflowID, step.payload)
		if err != nil {
			return err
		}
		var jobID string
		if err = tx.QueryRow(ctx, `INSERT INTO jobs(type,queue,payload,status,priority,max_attempts,timeout_seconds,workflow_id,workflow_step_key,owner_id) VALUES($1,$2,$3,'queued',$4,$5,$6,$7,$8,(SELECT owner_id FROM workflows WHERE id=$7)) RETURNING id`, step.jobType, step.queue, step.payload, step.priority, step.maxAttempts, step.timeout, step.workflowID, step.key).Scan(&jobID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE workflow_steps SET status='queued',job_id=$3,updated_at=now() WHERE workflow_id=$1 AND step_key=$2`, step.workflowID, step.key, jobID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO outbox(job_id,queue,priority) VALUES($1,$2,$3)`, jobID, step.queue, step.priority); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE workflows w SET status=CASE WHEN EXISTS(SELECT 1 FROM workflow_steps s WHERE s.workflow_id=w.id AND s.status='failed' AND NOT s.continue_on_error) THEN 'failed'::workflow_status ELSE 'succeeded'::workflow_status END,updated_at=now(),finished_at=now() WHERE w.id=(SELECT workflow_id FROM jobs WHERE id=$1) AND w.status='running' AND NOT EXISTS(SELECT 1 FROM workflow_steps s WHERE s.workflow_id=w.id AND s.status IN ('pending','queued','running'))`, job.ID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func evaluateCondition(ctx context.Context, tx pgx.Tx, workflowID, stepKey, condition string) (bool, error) {
	var failed int
	err := tx.QueryRow(ctx, `SELECT count(*) FROM workflow_steps target JOIN unnest(target.dependencies) d(key) ON true JOIN workflow_steps dep ON dep.workflow_id=target.workflow_id AND dep.step_key=d.key WHERE target.workflow_id=$1 AND target.step_key=$2 AND dep.status='failed' AND NOT dep.continue_on_error`, workflowID, stepKey).Scan(&failed)
	if err != nil {
		return false, err
	}
	switch strings.TrimSpace(strings.ToLower(condition)) {
	case "", "success()":
		return failed == 0, nil
	case "failure()":
		return failed > 0, nil
	case "always()":
		return true, nil
	default:
		return false, fmt.Errorf("unsupported workflow condition %q", condition)
	}
}

func expandFanOut(ctx context.Context, tx pgx.Tx, workflowID, key, jobType, queue string, payload []byte, priority int16, maxAttempts, timeout int, source string) error {
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT output FROM workflow_steps WHERE workflow_id=$1 AND step_key=$2 AND status='succeeded'`, workflowID, source).Scan(&raw); err != nil {
		return err
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return fmt.Errorf("fan_out_from %q must return a JSON array: %w", source, err)
	}
	if _, err := tx.Exec(ctx, `UPDATE workflow_steps SET status='running',updated_at=now() WHERE workflow_id=$1 AND step_key=$2 AND status='pending'`, workflowID, key); err != nil {
		return err
	}
	if len(items) == 0 {
		_, err := tx.Exec(ctx, `UPDATE workflow_steps SET status='succeeded',output='[]'::jsonb,updated_at=now() WHERE workflow_id=$1 AND step_key=$2`, workflowID, key)
		return err
	}
	for index, item := range items {
		childKey := fmt.Sprintf("%s[%d]", key, index)
		childPayload, err := replaceFanOutItem(payload, item)
		if err != nil {
			return err
		}
		var jobID string
		if err = tx.QueryRow(ctx, `INSERT INTO jobs(type,queue,payload,status,priority,max_attempts,timeout_seconds,workflow_id,workflow_step_key,owner_id) VALUES($1,$2,$3,'queued',$4,$5,$6,$7,$8,(SELECT owner_id FROM workflows WHERE id=$7)) RETURNING id`, jobType, queue, childPayload, priority, maxAttempts, timeout, workflowID, childKey).Scan(&jobID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO workflow_steps(workflow_id,step_key,job_type,queue,payload,status,priority,max_attempts,timeout_seconds,job_id,parent_step_key) VALUES($1,$2,$3,$4,$5,'queued',$6,$7,$8,$9,$10)`, workflowID, childKey, jobType, queue, childPayload, priority, maxAttempts, timeout, jobID, key); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO outbox(job_id,queue,priority) VALUES($1,$2,$3)`, jobID, queue, priority); err != nil {
			return err
		}
	}
	return nil
}
func replaceFanOutItem(payload []byte, item json.RawMessage) ([]byte, error) {
	var root, value any
	if err := json.Unmarshal(payload, &root); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(item, &value); err != nil {
		return nil, err
	}
	var walk func(any) any
	walk = func(v any) any {
		switch x := v.(type) {
		case string:
			if x == "${{ item }}" {
				return value
			}
		case map[string]any:
			for k, v := range x {
				x[k] = walk(v)
			}
		case []any:
			for i, v := range x {
				x[i] = walk(v)
			}
		}
		return v
	}
	return json.Marshal(walk(root))
}

func resolvePayload(ctx context.Context, tx pgx.Tx, workflowID string, payload []byte) ([]byte, error) {
	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		return nil, err
	}
	var resolve func(any) (any, error)
	resolve = func(v any) (any, error) {
		switch x := v.(type) {
		case string:
			const prefix = "${{ steps."
			if !strings.HasPrefix(x, prefix) || !strings.HasSuffix(x, " }}") {
				return x, nil
			}
			path := strings.Split(strings.TrimSuffix(strings.TrimPrefix(x, prefix), " }}"), ".output.")
			if len(path) != 2 {
				return nil, fmt.Errorf("invalid output reference %q", x)
			}
			var raw []byte
			if err := tx.QueryRow(ctx, `SELECT output FROM workflow_steps WHERE workflow_id=$1 AND step_key=$2`, workflowID, path[0]).Scan(&raw); err != nil {
				return nil, err
			}
			var output any
			if err := json.Unmarshal(raw, &output); err != nil {
				return nil, err
			}
			for _, key := range strings.Split(path[1], ".") {
				object, ok := output.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("output path %q is not an object", path[1])
				}
				output, ok = object[key]
				if !ok {
					return nil, fmt.Errorf("output path %q is missing", path[1])
				}
			}
			return output, nil
		case map[string]any:
			for key, item := range x {
				resolved, err := resolve(item)
				if err != nil {
					return nil, err
				}
				x[key] = resolved
			}
		case []any:
			for i, item := range x {
				resolved, err := resolve(item)
				if err != nil {
					return nil, err
				}
				x[i] = resolved
			}
		}
		return v, nil
	}
	resolved, err := resolve(value)
	if err != nil {
		return nil, err
	}
	return json.Marshal(resolved)
}
func (s Store) Fail(ctx context.Context, j Job, cause error) error {
	if cause == nil {
		cause = errors.New("unknown failure")
	}
	status := "retrying"
	var when any = time.Now().Add(retryDelay(j))
	if j.Attempt >= j.MaxAttempts || nonRetryable(j, cause) {
		status = "dead"
		when = j.ScheduledAt
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE jobs SET status=$3::job_status,last_error=$4,scheduled_at=$5,lease_expires_at=NULL,updated_at=now(),finished_at=CASE WHEN $3::text='dead' THEN now() ELSE NULL END WHERE id=$1 AND lease_token=$2 AND status='running'`, j.ID, j.LeaseToken, status, cause.Error(), when)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrLeaseLost
	}
	if _, err = tx.Exec(ctx, `UPDATE job_attempts SET finished_at=now(),error=$2 WHERE job_id=$1 AND attempt=$3`, j.ID, cause.Error(), j.Attempt); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE workflow_steps SET status=CASE WHEN $2='dead' THEN 'failed'::workflow_step_status ELSE 'queued'::workflow_step_status END,updated_at=now() WHERE job_id=$1`, j.ID, status); err != nil {
		return err
	}
	if status == "dead" {
		if _, err = tx.Exec(ctx, `UPDATE workflows SET status=CASE WHEN EXISTS(SELECT 1 FROM workflow_steps s WHERE s.workflow_id=workflows.id AND s.status='succeeded' AND s.compensation IS NOT NULL) THEN 'compensating'::workflow_status ELSE 'failed'::workflow_status END,updated_at=now(),finished_at=CASE WHEN EXISTS(SELECT 1 FROM workflow_steps s WHERE s.workflow_id=workflows.id AND s.status='succeeded' AND s.compensation IS NOT NULL) THEN NULL ELSE now() END WHERE id=(SELECT workflow_id FROM jobs WHERE id=$1) AND fail_fast`, j.ID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE workflow_steps SET status='cancelled',updated_at=now() WHERE workflow_id=(SELECT workflow_id FROM jobs WHERE id=$1) AND status IN ('pending','queued') AND EXISTS(SELECT 1 FROM workflows w WHERE w.id=workflow_id AND w.fail_fast)`, j.ID); err != nil {
			return err
		}
		if err = startCompensation(ctx, tx, j.ID); err != nil {
			return err
		}
		if err = releaseAfterFailure(ctx, tx, j.ID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func startCompensation(ctx context.Context, tx pgx.Tx, failedJobID string) error {
	rows, err := tx.Query(ctx, `SELECT s.workflow_id,s.step_key,s.compensation FROM workflow_steps s JOIN workflows w ON w.id=s.workflow_id WHERE s.workflow_id=(SELECT workflow_id FROM jobs WHERE id=$1) AND w.status='compensating' AND s.status='succeeded' AND s.compensation IS NOT NULL ORDER BY s.created_at DESC FOR UPDATE OF s`, failedJobID)
	if err != nil {
		return err
	}
	type item struct {
		workflowID, key string
		comp            []byte
	}
	var items []item
	for rows.Next() {
		var x item
		if err = rows.Scan(&x.workflowID, &x.key, &x.comp); err != nil {
			rows.Close()
			return err
		}
		items = append(items, x)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, x := range items {
		var spec struct {
			Type, Queue string
			Payload     json.RawMessage
		}
		if err = json.Unmarshal(x.comp, &spec); err != nil {
			return err
		}
		if spec.Queue == "" {
			spec.Queue = "jobs.default"
		}
		key := "compensate:" + x.key
		var jobID string
		if err = tx.QueryRow(ctx, `INSERT INTO jobs(type,queue,payload,status,max_attempts,workflow_id,workflow_step_key,owner_id) VALUES($1,$2,$3,'queued',3,$4,$5,(SELECT owner_id FROM workflows WHERE id=$4)) RETURNING id`, spec.Type, spec.Queue, spec.Payload, x.workflowID, key).Scan(&jobID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO workflow_steps(workflow_id,step_key,job_type,queue,payload,status,job_id,is_compensation,parent_step_key) VALUES($1,$2,$3,$4,$5,'queued',$6,true,$7) ON CONFLICT DO NOTHING`, x.workflowID, key, spec.Type, spec.Queue, spec.Payload, jobID, x.key); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO outbox(job_id,queue,priority) VALUES($1,$2,9)`, jobID, spec.Queue); err != nil {
			return err
		}
	}
	return nil
}

func releaseAfterFailure(ctx context.Context, tx pgx.Tx, failedJobID string) error {
	rows, err := tx.Query(ctx, `SELECT s.workflow_id,s.step_key,s.job_type,s.queue,s.payload,s.priority,s.max_attempts,s.timeout_seconds,COALESCE(s.condition,'success()') FROM workflow_steps s JOIN workflows w ON w.id=s.workflow_id WHERE s.workflow_id=(SELECT workflow_id FROM jobs WHERE id=$1) AND NOT w.fail_fast AND s.status='pending' AND NOT EXISTS (SELECT 1 FROM unnest(s.dependencies) d LEFT JOIN workflow_steps dep ON dep.workflow_id=s.workflow_id AND dep.step_key=d WHERE dep.status NOT IN ('succeeded','failed','skipped')) FOR UPDATE OF s`, failedJobID)
	if err != nil {
		return err
	}
	type readyStep struct {
		workflowID, key, jobType, queue string
		payload                         []byte
		priority                        int16
		maxAttempts, timeout            int
		condition                       string
	}
	var ready []readyStep
	for rows.Next() {
		var s readyStep
		if err = rows.Scan(&s.workflowID, &s.key, &s.jobType, &s.queue, &s.payload, &s.priority, &s.maxAttempts, &s.timeout, &s.condition); err != nil {
			rows.Close()
			return err
		}
		ready = append(ready, s)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, s := range ready {
		run, e := evaluateCondition(ctx, tx, s.workflowID, s.key, s.condition)
		if e != nil {
			return e
		}
		if !run {
			if _, e = tx.Exec(ctx, `UPDATE workflow_steps SET status='skipped',updated_at=now() WHERE workflow_id=$1 AND step_key=$2`, s.workflowID, s.key); e != nil {
				return e
			}
			continue
		}
		s.payload, e = resolvePayload(ctx, tx, s.workflowID, s.payload)
		if e != nil {
			return e
		}
		var jobID string
		if e = tx.QueryRow(ctx, `INSERT INTO jobs(type,queue,payload,status,priority,max_attempts,timeout_seconds,workflow_id,workflow_step_key,owner_id) VALUES($1,$2,$3,'queued',$4,$5,$6,$7,$8,(SELECT owner_id FROM workflows WHERE id=$7)) RETURNING id`, s.jobType, s.queue, s.payload, s.priority, s.maxAttempts, s.timeout, s.workflowID, s.key).Scan(&jobID); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE workflow_steps SET status='queued',job_id=$3,updated_at=now() WHERE workflow_id=$1 AND step_key=$2`, s.workflowID, s.key, jobID); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO outbox(job_id,queue,priority) VALUES($1,$2,$3)`, jobID, s.queue, s.priority); e != nil {
			return e
		}
	}
	return nil
}
func (s Store) RecoverExpired(ctx context.Context) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE jobs SET status='cancelled',lease_expires_at=NULL,finished_at=now(),updated_at=now() WHERE status='cancelling' AND lease_expires_at<now()`); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE jobs SET status='retrying',scheduled_at=now(),lease_expires_at=NULL,last_error='worker lease expired',updated_at=now() WHERE status='running' AND lease_expires_at<now()`); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s Store) RenewLease(ctx context.Context, id, worker string, token int64, lease time.Duration) error {
	tag, err := s.DB.Exec(ctx, `UPDATE jobs SET lease_expires_at=now()+$4::interval,updated_at=now() WHERE id=$1 AND worker_id=$2 AND lease_token=$3 AND status IN ('running','cancelling')`, id, worker, token, lease.String())
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
func (s Store) CancellationRequested(ctx context.Context, id string, token int64) (bool, string, error) {
	var requested bool
	var reason *string
	err := s.DB.QueryRow(ctx, `SELECT cancellation_requested_at IS NOT NULL,cancellation_reason FROM jobs WHERE id=$1 AND lease_token=$2`, id, token).Scan(&requested, &reason)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, "", ErrLeaseLost
	}
	if err != nil {
		return false, "", err
	}
	if reason == nil {
		return requested, "", nil
	}
	return requested, *reason, nil
}
func (s Store) CompleteCancellation(ctx context.Context, j Job, reason string) error {
	if reason == "" {
		reason = "cancelled by user"
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE jobs SET status='cancelled',cancellation_reason=$3,lease_expires_at=NULL,updated_at=now(),finished_at=now() WHERE id=$1 AND lease_token=$2 AND status='cancelling'`, j.ID, j.LeaseToken, reason)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrLeaseLost
	}
	if _, err = tx.Exec(ctx, `UPDATE job_attempts SET finished_at=now(),error=$3 WHERE job_id=$1 AND lease_token=$2`, j.ID, j.LeaseToken, "cancelled: "+reason); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE workflow_steps SET status='cancelled',updated_at=now() WHERE job_id=$1`, j.ID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s Store) HeartbeatWorker(ctx context.Context, id string, queues, capabilities []string, concurrency int) error {
	queue := ""
	if len(queues) > 0 {
		queue = queues[0]
	}
	_, err := s.DB.Exec(ctx, `INSERT INTO workers(id,queue,queues,capabilities,concurrency,last_heartbeat_at,state) VALUES($1,$2,$3,$4,$5,now(),'running') ON CONFLICT(id) DO UPDATE SET queue=EXCLUDED.queue,queues=EXCLUDED.queues,capabilities=EXCLUDED.capabilities,concurrency=EXCLUDED.concurrency,last_heartbeat_at=now(),state='running',draining_at=NULL`, id, queue, queues, capabilities, concurrency)
	return err
}
func (s Store) SetWorkerState(ctx context.Context, id, state string) error {
	_, err := s.DB.Exec(ctx, `UPDATE workers SET state=$2,draining_at=CASE WHEN $2='draining' THEN now() ELSE draining_at END,last_heartbeat_at=now() WHERE id=$1`, id, state)
	return err
}
func (s Store) ReleaseWorkerJobs(ctx context.Context, workerID string) (int64, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `UPDATE jobs SET status=CASE WHEN status='cancelling' THEN 'cancelled'::job_status ELSE 'retrying'::job_status END,worker_id=NULL,lease_expires_at=NULL,scheduled_at=now(),updated_at=now(),finished_at=CASE WHEN status='cancelling' THEN now() ELSE NULL END,last_error=CASE WHEN status='running' THEN 'worker drain timeout' ELSE last_error END WHERE worker_id=$1 AND status IN ('running','cancelling') RETURNING id,lease_token`, workerID)
	if err != nil {
		return 0, err
	}
	type releasedJob struct {
		id    string
		token int64
	}
	var released []releasedJob
	for rows.Next() {
		var id string
		var token int64
		if err = rows.Scan(&id, &token); err != nil {
			rows.Close()
			return 0, err
		}
		released = append(released, releasedJob{id: id, token: token})
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return 0, err
	}
	for _, job := range released {
		if _, err = tx.Exec(ctx, `UPDATE job_attempts SET finished_at=now(),error='worker drain timeout' WHERE job_id=$1 AND lease_token=$2 AND finished_at IS NULL`, job.id, job.token); err != nil {
			return 0, err
		}
	}
	return int64(len(released)), tx.Commit(ctx)
}
func retryDelay(j Job) time.Duration {
	initial, maximum := float64(j.RetryInitialSeconds), float64(j.RetryMaxSeconds)
	if initial <= 0 {
		initial = 2
	}
	if maximum <= 0 {
		maximum = 300
	}
	multiplier := 1.0
	switch j.RetryPolicy {
	case "fixed":
	case "linear":
		multiplier = float64(j.Attempt)
	default:
		multiplier = math.Pow(2, float64(j.Attempt-1))
	}
	seconds := math.Min(maximum, initial*multiplier)
	if j.RetryJitter {
		seconds *= .5 + rand.Float64()
	}
	return time.Duration(seconds * float64(time.Second))
}

func nonRetryable(j Job, cause error) bool {
	var permanent interface{ Permanent() bool }
	if errors.As(cause, &permanent) && permanent.Permanent() {
		return true
	}
	message := strings.ToLower(cause.Error())
	for _, pattern := range j.NonRetryableErrors {
		if pattern != "" && strings.Contains(message, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}
