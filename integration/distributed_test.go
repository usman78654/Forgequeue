package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/usman78654/Forgequeue/internal/broker"
	"github.com/usman78654/Forgequeue/internal/jobs"
	"github.com/usman78654/Forgequeue/internal/schedules"
	"github.com/usman78654/Forgequeue/internal/workflows"
	"github.com/jackc/pgx/v5/pgxpool"
)

func database(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	return ctx, db
}
func unique(prefix string) string { return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano()) }

func TestIdempotencyAndDuplicateClaim(t *testing.T) {
	ctx, db := database(t)
	store := jobs.Store{DB: db}
	key := unique("idempotent")
	req := jobs.SubmitRequest{Type: "echo", Queue: unique("test.queue"), Payload: json.RawMessage(`{"ok":true}`), IdempotencyKey: key}
	first, created, err := store.Submit(ctx, req)
	if err != nil || !created {
		t.Fatalf("first submission created=%v err=%v", created, err)
	}
	defer db.Exec(ctx, `DELETE FROM jobs WHERE id=$1`, first.ID)
	second, created, err := store.Submit(ctx, req)
	if err != nil || created || second.ID != first.ID {
		t.Fatalf("duplicate submission created=%v same=%v err=%v", created, second.ID == first.ID, err)
	}
	claimed, err := store.Claim(ctx, first.ID, "integration-worker", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Claim(ctx, first.ID, "duplicate-worker", time.Minute); !errors.Is(err, jobs.ErrNotFound) {
		t.Fatalf("duplicate claim error=%v", err)
	}
	if err = store.Succeed(ctx, claimed, []byte(`{"done":true}`)); err != nil {
		t.Fatal(err)
	}
}

func TestRetryExhaustionAndDeadLetter(t *testing.T) {
	ctx, db := database(t)
	store := jobs.Store{DB: db}
	job, _, err := store.Submit(ctx, jobs.SubmitRequest{Type: "always-fails", Queue: unique("test.queue"), Payload: json.RawMessage(`{}`), MaxAttempts: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM jobs WHERE id=$1`, job.ID)
	first, err := store.Claim(ctx, job.ID, "worker-1", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Fail(ctx, first, errors.New("first failure")); err != nil {
		t.Fatal(err)
	}
	_, _ = db.Exec(ctx, `UPDATE jobs SET scheduled_at=now() WHERE id=$1`, job.ID)
	if err = store.PromoteScheduled(ctx); err != nil {
		t.Fatal(err)
	}
	second, err := store.Claim(ctx, job.ID, "worker-2", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Fail(ctx, second, errors.New("final failure")); err != nil {
		t.Fatal(err)
	}
	final, err := store.Get(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != "dead" || final.Attempt != 2 {
		t.Fatalf("status=%s attempt=%d", final.Status, final.Attempt)
	}
}

func TestCrashRecoveryAndLeaseFencing(t *testing.T) {
	ctx, db := database(t)
	store := jobs.Store{DB: db}
	job, _, err := store.Submit(ctx, jobs.SubmitRequest{Type: "echo", Queue: unique("test.queue"), Payload: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM jobs WHERE id=$1`, job.ID)
	stale, err := store.Claim(ctx, job.ID, "crashed-worker", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = db.Exec(ctx, `UPDATE jobs SET lease_expires_at=now()-interval '1 second' WHERE id=$1`, job.ID)
	if err = store.RecoverExpired(ctx); err != nil {
		t.Fatal(err)
	}
	if err = store.PromoteScheduled(ctx); err != nil {
		t.Fatal(err)
	}
	current, err := store.Claim(ctx, job.ID, "recovery-worker", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if current.LeaseToken <= stale.LeaseToken {
		t.Fatal("lease token did not advance")
	}
	if err = store.Succeed(ctx, stale, []byte(`{"stale":true}`)); !errors.Is(err, jobs.ErrLeaseLost) {
		t.Fatalf("stale completion error=%v", err)
	}
	if err = store.Succeed(ctx, current, []byte(`{"recovered":true}`)); err != nil {
		t.Fatal(err)
	}
}

func TestWorkflowDependencyRelease(t *testing.T) {
	ctx, db := database(t)
	jobStore := jobs.Store{DB: db}
	workflowStore := workflows.Store{DB: db}
	queue := unique("test.workflow")
	workflow, err := workflowStore.Submit(ctx, workflows.SubmitRequest{Name: "integration-dag", Steps: []workflows.StepRequest{{Key: "root", Type: "echo", Queue: queue, Payload: json.RawMessage(`{"step":1}`)}, {Key: "child", Type: "echo", Queue: queue, Payload: json.RawMessage(`{"step":2}`), DependsOn: []string{"root"}}}})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM workflows WHERE id=$1`, workflow.ID)
	var rootID string
	for _, step := range workflow.Steps {
		if step.Key == "root" && step.JobID != nil {
			rootID = *step.JobID
		}
	}
	if rootID == "" {
		t.Fatal("root job was not created")
	}
	rootJob, err := jobStore.Claim(ctx, rootID, "workflow-worker", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err = jobStore.Succeed(ctx, rootJob, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	workflow, err = workflowStore.Get(ctx, workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	var childID string
	for _, step := range workflow.Steps {
		if step.Key == "child" && step.JobID != nil {
			childID = *step.JobID
		}
	}
	if childID == "" {
		t.Fatal("dependent job was not released")
	}
	child, err := jobStore.Claim(ctx, childID, "workflow-worker", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err = jobStore.Succeed(ctx, child, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	workflow, err = workflowStore.Get(ctx, workflow.ID)
	if err != nil || workflow.Status != "succeeded" {
		t.Fatalf("workflow status=%s err=%v", workflow.Status, err)
	}
}

func TestTransactionalOutboxToRabbitMQ(t *testing.T) {
	url := os.Getenv("TEST_RABBIT_URL")
	if url == "" {
		t.Skip("TEST_RABBIT_URL is not set")
	}
	ctx, db := database(t)
	rabbit, err := broker.Open(url)
	if err != nil {
		t.Fatal(err)
	}
	defer rabbit.Close()
	queue := unique("test.outbox")
	consumeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	deliveries := rabbit.Consume(consumeCtx, queue, "integration-consumer", 1)
	relayCtx, stopRelay := context.WithCancel(ctx)
	defer stopRelay()
	go broker.RunOutbox(relayCtx, db, rabbit, 25*time.Millisecond)
	store := jobs.Store{DB: db}
	job, _, err := store.Submit(ctx, jobs.SubmitRequest{Type: "echo", Queue: queue, Payload: json.RawMessage(`{"outbox":true}`)})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM jobs WHERE id=$1`, job.ID)
	select {
	case delivery := <-deliveries:
		var message broker.Message
		if err = json.Unmarshal(delivery.Body, &message); err != nil {
			t.Fatal(err)
		}
		if message.JobID != job.ID {
			t.Fatalf("delivered job %s want %s", message.JobID, job.ID)
		}
		_ = delivery.Ack(false)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for outbox delivery")
	}
}

func TestCursorPaginationAndFilters(t *testing.T) {
	ctx, db := database(t)
	store := jobs.Store{DB: db}
	queue := unique("test.pagination")
	var ids []string
	for i := 0; i < 5; i++ {
		job, _, err := store.Submit(ctx, jobs.SubmitRequest{Type: "page-type", Queue: queue, Payload: json.RawMessage(fmt.Sprintf(`{"index":%d}`, i))})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, job.ID)
		time.Sleep(time.Millisecond)
	}
	defer func() {
		for _, id := range ids {
			_, _ = db.Exec(ctx, `DELETE FROM jobs WHERE id=$1`, id)
		}
	}()
	first, err := store.List(ctx, jobs.ListFilter{Queue: queue, Type: "page-type", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 2 || first.NextCursor == "" {
		t.Fatalf("first page items=%d cursor=%q", len(first.Items), first.NextCursor)
	}
	second, err := store.List(ctx, jobs.ListFilter{Queue: queue, Type: "page-type", Limit: 2, Cursor: first.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 2 || second.NextCursor == "" {
		t.Fatalf("second page items=%d cursor=%q", len(second.Items), second.NextCursor)
	}
	third, err := store.List(ctx, jobs.ListFilter{Queue: queue, Type: "page-type", Limit: 2, Cursor: second.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(third.Items) != 1 || third.NextCursor != "" {
		t.Fatalf("third page items=%d cursor=%q", len(third.Items), third.NextCursor)
	}
	seen := map[string]bool{}
	for _, page := range []jobs.Page{first, second, third} {
		for _, job := range page.Items {
			if seen[job.ID] {
				t.Fatalf("duplicate job %s across pages", job.ID)
			}
			seen[job.ID] = true
		}
	}
	if len(seen) != 5 {
		t.Fatalf("saw %d jobs, want 5", len(seen))
	}
}

func TestJobEventRetentionAndPaging(t *testing.T) {
	ctx, db := database(t)
	store := jobs.Store{DB: db}
	job, _, err := store.Submit(ctx, jobs.SubmitRequest{Type: "echo", Queue: unique("test.events"), Payload: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM jobs WHERE id=$1`, job.ID)
	if _, err = db.Exec(ctx, `INSERT INTO job_events(job_id,event_type,message) SELECT $1,'log','event-'||n FROM generate_series(1,$2) n`, job.ID, jobs.MaxEventsPerJob+5); err != nil {
		t.Fatal(err)
	}
	latest, err := store.AppendEvent(ctx, job.ID, jobs.EventInput{Type: "progress", Message: "latest", Progress: floatPointer(.75)})
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM job_events WHERE job_id=$1`, job.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != jobs.MaxEventsPerJob {
		t.Fatalf("retained %d events want %d", count, jobs.MaxEventsPerJob)
	}
	first, err := store.ListEvents(ctx, job.ID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 10 || first.NextAfter == 0 || !first.HasMore {
		t.Fatalf("page items=%d next=%d", len(first.Items), first.NextAfter)
	}
	last, err := store.ListEvents(ctx, job.ID, latest.ID-1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(last.Items) != 1 || last.Items[0].Message != "latest" {
		t.Fatalf("latest page=%+v", last)
	}
}
func floatPointer(value float64) *float64 { return &value }

func TestWorkflowOutputsConditionsAndContinueOnError(t *testing.T) {
	ctx, db := database(t)
	workflowStore, jobStore := workflows.Store{DB: db}, jobs.Store{DB: db}
	workflow, err := workflowStore.Submit(ctx, workflows.SubmitRequest{Name: unique("outputs"), Steps: []workflows.StepRequest{
		{Key: "source", Type: "echo", Payload: json.RawMessage(`{"value":"hello"}`)},
		{Key: "consume", Type: "echo", Payload: json.RawMessage(`{"received":"${{ steps.source.output.value }}"}`), DependsOn: []string{"source"}, Condition: "success()"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM workflows WHERE id=$1`, workflow.ID)
	current, _ := workflowStore.Get(ctx, workflow.ID)
	var sourceID string
	for _, step := range current.Steps {
		if step.Key == "source" && step.JobID != nil {
			sourceID = *step.JobID
		}
	}
	source, err := jobStore.Claim(ctx, sourceID, "output-test", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err = jobStore.Succeed(ctx, source, []byte(`{"value":"hello"}`)); err != nil {
		t.Fatal(err)
	}
	current, err = workflowStore.Get(ctx, workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	var consumeID string
	for _, step := range current.Steps {
		if step.Key == "consume" && step.JobID != nil {
			consumeID = *step.JobID
		}
	}
	consume, err := jobStore.Get(ctx, consumeID)
	if err != nil {
		t.Fatal(err)
	}
	if string(consume.Payload) != `{"received": "hello"}` && string(consume.Payload) != `{"received":"hello"}` {
		t.Fatalf("resolved payload=%s", consume.Payload)
	}

	failFast := false
	continued, err := workflowStore.Submit(ctx, workflows.SubmitRequest{Name: unique("continue"), FailFast: &failFast, Steps: []workflows.StepRequest{
		{Key: "optional", Type: "missing", Payload: json.RawMessage(`{}`), MaxAttempts: 1, ContinueOnError: true},
		{Key: "finish", Type: "echo", Payload: json.RawMessage(`{"ok":true}`), DependsOn: []string{"optional"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM workflows WHERE id=$1`, continued.ID)
	continued, _ = workflowStore.Get(ctx, continued.ID)
	var optionalID string
	for _, step := range continued.Steps {
		if step.Key == "optional" && step.JobID != nil {
			optionalID = *step.JobID
		}
	}
	failed, err := jobStore.Claim(ctx, optionalID, "continue-test", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err = jobStore.Fail(ctx, failed, errors.New("expected failure")); err != nil {
		t.Fatal(err)
	}
	continued, _ = workflowStore.Get(ctx, continued.ID)
	var finishID string
	for _, step := range continued.Steps {
		if step.Key == "finish" && step.JobID != nil {
			finishID = *step.JobID
		}
	}
	finish, err := jobStore.Claim(ctx, finishID, "continue-test", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err = jobStore.Succeed(ctx, finish, []byte(`{"ok":true}`)); err != nil {
		t.Fatal(err)
	}
	continued, _ = workflowStore.Get(ctx, continued.ID)
	if continued.Status != "succeeded" {
		t.Fatalf("continued workflow status=%s", continued.Status)
	}
}

func TestDynamicFanOutFanInAndCompensation(t *testing.T) {
	ctx, db := database(t)
	ws, js := workflows.Store{DB: db}, jobs.Store{DB: db}
	wf, err := ws.Submit(ctx, workflows.SubmitRequest{Name: unique("fanout"), Steps: []workflows.StepRequest{{Key: "source", Type: "echo", Payload: json.RawMessage(`[{"n":1},{"n":2}]`)}, {Key: "map", Type: "echo", Payload: json.RawMessage(`{"item":"${{ item }}"}`), FanOutFrom: "source"}, {Key: "join", Type: "echo", Payload: json.RawMessage(`{}`), DependsOn: []string{"map"}}}})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM workflows WHERE id=$1`, wf.ID)
	wf, _ = ws.Get(ctx, wf.ID)
	var sourceID string
	for _, s := range wf.Steps {
		if s.Key == "source" {
			sourceID = *s.JobID
		}
	}
	source, err := js.Claim(ctx, sourceID, "fanout-test", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err = js.Succeed(ctx, source, []byte(`[{"n":1},{"n":2}]`)); err != nil {
		t.Fatal(err)
	}
	wf, _ = ws.Get(ctx, wf.ID)
	children := 0
	for _, s := range wf.Steps {
		if s.ParentStepKey == "map" {
			children++
			child, e := js.Claim(ctx, *s.JobID, "fanout-test", time.Minute)
			if e != nil {
				t.Fatal(e)
			}
			if e = js.Succeed(ctx, child, s.Output); e != nil {
				t.Fatal(e)
			}
		}
	}
	if children != 2 {
		t.Fatalf("fanout children=%d", children)
	}
	wf, _ = ws.Get(ctx, wf.ID)
	var joinID string
	for _, s := range wf.Steps {
		if s.Key == "join" && s.JobID != nil {
			joinID = *s.JobID
		}
	}
	if joinID == "" {
		t.Fatal("fan-in was not released")
	}

	comp := &workflows.CompensationRequest{Type: "echo", Payload: json.RawMessage(`{"undo":true}`)}
	saga, err := ws.Submit(ctx, workflows.SubmitRequest{Name: unique("saga"), Steps: []workflows.StepRequest{{Key: "reserve", Type: "echo", Payload: json.RawMessage(`{}`), Compensation: comp}, {Key: "fail", Type: "missing", Payload: json.RawMessage(`{}`), MaxAttempts: 1, DependsOn: []string{"reserve"}}}})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM workflows WHERE id=$1`, saga.ID)
	saga, _ = ws.Get(ctx, saga.ID)
	var reserveID string
	for _, s := range saga.Steps {
		if s.Key == "reserve" {
			reserveID = *s.JobID
		}
	}
	reserve, _ := js.Claim(ctx, reserveID, "saga-test", time.Minute)
	if err = js.Succeed(ctx, reserve, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	saga, _ = ws.Get(ctx, saga.ID)
	var failID string
	for _, s := range saga.Steps {
		if s.Key == "fail" {
			failID = *s.JobID
		}
	}
	failed, _ := js.Claim(ctx, failID, "saga-test", time.Minute)
	if err = js.Fail(ctx, failed, errors.New("boom")); err != nil {
		t.Fatal(err)
	}
	saga, _ = ws.Get(ctx, saga.ID)
	var compensationID string
	for _, s := range saga.Steps {
		if s.IsCompensation {
			compensationID = *s.JobID
		}
	}
	if compensationID == "" {
		t.Fatal("compensation was not scheduled")
	}
	undo, _ := js.Claim(ctx, compensationID, "saga-test", time.Minute)
	if err = js.Succeed(ctx, undo, []byte(`{"undone":true}`)); err != nil {
		t.Fatal(err)
	}
	saga, _ = ws.Get(ctx, saga.ID)
	if saga.Status != "compensated" {
		t.Fatalf("saga status=%s", saga.Status)
	}
}

func TestQueueRoutingCapabilitiesAndTenantQuota(t *testing.T) {
	ctx, db := database(t)
	store := jobs.Store{DB: db}
	queue := unique("routing")
	_, err := db.Exec(ctx, `INSERT INTO queue_controls(queue,paused,max_concurrency) VALUES($1,true,1)`, queue)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM queue_controls WHERE queue=$1`, queue)
	job, _, err := store.Submit(ctx, jobs.SubmitRequest{Type: "echo", Queue: queue, Payload: json.RawMessage(`{}`), RequiredCapabilities: []string{"gpu"}})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM jobs WHERE id=$1`, job.ID)
	if _, err = store.Claim(ctx, job.ID, "cpu-worker", time.Minute, "cpu"); !errors.Is(err, jobs.ErrRoutingBlocked) {
		t.Fatalf("paused claim error=%v", err)
	}
	_, _ = db.Exec(ctx, `UPDATE queue_controls SET paused=false WHERE queue=$1`, queue)
	if _, err = store.Claim(ctx, job.ID, "cpu-worker", time.Minute, "cpu"); !errors.Is(err, jobs.ErrRoutingBlocked) {
		t.Fatalf("capability claim error=%v", err)
	}
	if _, err = store.Claim(ctx, job.ID, "gpu-worker", time.Minute, "gpu"); err != nil {
		t.Fatal(err)
	}
	var principal string
	if err = db.QueryRow(ctx, `INSERT INTO principals(name,role) VALUES($1,'operator') RETURNING id`, unique("quota")).Scan(&principal); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM principals WHERE id=$1`, principal)
	_, _ = db.Exec(ctx, `INSERT INTO tenant_quotas(principal_id,submissions_per_minute,max_concurrent_jobs,max_payload_bytes,max_schedules) VALUES($1,1,10,64,1)`, principal)
	quotaJob, _, err := store.Submit(ctx, jobs.SubmitRequest{Type: "echo", Payload: json.RawMessage(`{}`), OwnerID: principal})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM jobs WHERE id=$1`, quotaJob.ID)
	if _, _, err = store.Submit(ctx, jobs.SubmitRequest{Type: "echo", Payload: json.RawMessage(`{}`), OwnerID: principal}); !errors.Is(err, jobs.ErrQuotaExceeded) {
		t.Fatalf("quota error=%v", err)
	}
	_, _ = db.Exec(ctx, `UPDATE tenant_quotas SET submissions_per_minute=100,max_concurrent_jobs=1,max_payload_bytes=64 WHERE principal_id=$1`, principal)
	if _, _, err = store.Submit(ctx, jobs.SubmitRequest{Type: "echo", Payload: json.RawMessage(`{}`), OwnerID: principal}); !errors.Is(err, jobs.ErrQuotaExceeded) {
		t.Fatalf("concurrency quota error=%v", err)
	}
	_, _ = db.Exec(ctx, `UPDATE tenant_quotas SET max_concurrent_jobs=10,max_payload_bytes=2,max_schedules=1 WHERE principal_id=$1`, principal)
	if _, _, err = store.Submit(ctx, jobs.SubmitRequest{Type: "echo", Payload: json.RawMessage(`{"too":"large"}`), OwnerID: principal}); !errors.Is(err, jobs.ErrQuotaExceeded) {
		t.Fatalf("payload quota error=%v", err)
	}
	scheduleStore := schedules.Store{DB: db}
	firstSchedule, err := scheduleStore.Create(ctx, schedules.CreateRequest{Name: unique("quota-schedule"), Type: "echo", Payload: json.RawMessage(`{}`), IntervalSeconds: 3600, OwnerID: principal})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM recurring_schedules WHERE id=$1`, firstSchedule.ID)
	if _, err = scheduleStore.Create(ctx, schedules.CreateRequest{Name: unique("quota-schedule"), Type: "echo", Payload: json.RawMessage(`{}`), IntervalSeconds: 3600, OwnerID: principal}); err == nil {
		t.Fatal("expected schedule quota error")
	}
}
