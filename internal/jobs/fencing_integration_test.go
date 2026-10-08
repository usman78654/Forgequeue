package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLeaseFencingIntegration(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := Store{DB: db}
	key := fmt.Sprintf("fencing-test-%d", time.Now().UnixNano())
	job, _, err := store.Submit(ctx, SubmitRequest{Type: "echo", Queue: "test.fencing", Payload: json.RawMessage(`{"test":true}`), IdempotencyKey: key})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(context.Background(), `DELETE FROM jobs WHERE id=$1`, job.ID)
	first, err := store.Claim(ctx, job.ID, "old-worker", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `UPDATE jobs SET lease_expires_at=now()-interval '1 second' WHERE id=$1`, job.ID); err != nil {
		t.Fatal(err)
	}
	if err = store.RecoverExpired(ctx); err != nil {
		t.Fatal(err)
	}
	if err = store.PromoteScheduled(ctx); err != nil {
		t.Fatal(err)
	}
	second, err := store.Claim(ctx, job.ID, "new-worker", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if second.LeaseToken <= first.LeaseToken {
		t.Fatalf("fencing token did not increase: first=%d second=%d", first.LeaseToken, second.LeaseToken)
	}
	if err = store.Succeed(ctx, first, []byte(`{"stale":true}`)); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale worker completion error=%v, want ErrLeaseLost", err)
	}
	if err = store.Succeed(ctx, second, []byte(`{"fresh":true}`)); err != nil {
		t.Fatalf("current worker could not complete: %v", err)
	}
	final, err := store.Get(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]bool
	if err = json.Unmarshal(final.Result, &result); err != nil {
		t.Fatal(err)
	}
	if !result["fresh"] || result["stale"] {
		t.Fatalf("unexpected final result %s", final.Result)
	}
}
