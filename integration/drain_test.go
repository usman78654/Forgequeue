package integration_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/usman78654/Forgequeue/internal/jobs"
)

func TestDrainReleaseIsIdempotentAndClosesAttempts(t *testing.T) {
	ctx, db := database(t)
	store := jobs.Store{DB: db}
	workerID := unique("draining-worker")
	queue := unique("drain-queue")
	if err := store.HeartbeatWorker(ctx, workerID, []string{queue}, []string{"echo"}, 2); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM workers WHERE id=$1`, workerID)
	job, _, err := store.Submit(ctx, jobs.SubmitRequest{Type: "echo", Queue: queue, Payload: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM jobs WHERE id=$1`, job.ID)
	claimed, err := store.Claim(ctx, job.ID, workerID, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetWorkerState(ctx, workerID, "draining"); err != nil {
		t.Fatal(err)
	}
	count, err := store.ReleaseWorkerJobs(ctx, workerID)
	if err != nil || count != 1 {
		t.Fatalf("release count=%d err=%v", count, err)
	}
	count, err = store.ReleaseWorkerJobs(ctx, workerID)
	if err != nil || count != 0 {
		t.Fatalf("repeat release count=%d err=%v", count, err)
	}
	if err := store.Succeed(ctx, claimed, []byte(`{}`)); !errors.Is(err, jobs.ErrLeaseLost) {
		t.Fatalf("stale completion accepted: %v", err)
	}
	attempts, err := store.Attempts(ctx, job.ID)
	if err != nil || len(attempts) != 1 || attempts[0].FinishedAt == nil || attempts[0].Error == nil || *attempts[0].Error != "worker drain timeout" {
		t.Fatalf("attempts=%+v err=%v", attempts, err)
	}
	current, err := store.Get(ctx, job.ID)
	if err != nil || current.Status != "retrying" {
		t.Fatalf("job=%+v err=%v", current, err)
	}
	if err := store.HeartbeatWorker(ctx, workerID, []string{queue}, []string{"echo"}, 2); err != nil {
		t.Fatal(err)
	}
	var state string
	var drainingAt *time.Time
	if err := db.QueryRow(ctx, `SELECT state,draining_at FROM workers WHERE id=$1`, workerID).Scan(&state, &drainingAt); err != nil || state != "running" || drainingAt != nil {
		t.Fatalf("restart state=%s draining=%v err=%v", state, drainingAt, err)
	}
}

func TestCompletedWorkIsNotReleasedDuringDrain(t *testing.T) {
	ctx, db := database(t)
	store := jobs.Store{DB: db}
	workerID := unique("completed-drain")
	job, _, err := store.Submit(ctx, jobs.SubmitRequest{Type: "echo", Queue: unique("drain"), Payload: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM jobs WHERE id=$1`, job.ID)
	claimed, err := store.Claim(ctx, job.ID, workerID, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Succeed(ctx, claimed, []byte(`{"ok":true}`)); err != nil {
		t.Fatal(err)
	}
	if count, err := store.ReleaseWorkerJobs(ctx, workerID); err != nil || count != 0 {
		t.Fatalf("completed job released: count=%d err=%v", count, err)
	}
	current, err := store.Get(ctx, job.ID)
	if err != nil || current.Status != "succeeded" {
		t.Fatalf("terminal state changed: job=%+v err=%v", current, err)
	}
}
