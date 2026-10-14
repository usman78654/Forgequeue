package integration_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/usman78654/Forgequeue/internal/artifacts"
	"github.com/usman78654/Forgequeue/internal/jobs"
)

// This tests PostgreSQL lifecycle ordering against controlled storage failures;
// the separate MinIO test verifies the real S3 protocol and signed URLs.
func TestArtifactCleanupFailurePreservesMetadata(t *testing.T) {
	ctx, db := database(t)
	var failing atomic.Bool
	failing.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && failing.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	store := artifacts.Store{DB: db, Endpoint: server.URL, AccessKey: "test", SecretKey: "test", Bucket: "test", MaxBytes: 1024, Retention: time.Hour}
	job, _, err := (jobs.Store{DB: db}).Submit(ctx, jobs.SubmitRequest{Type: "echo", Queue: unique("artifact-failure"), Payload: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM jobs WHERE id=$1`, job.ID)
	item, err := store.Put(ctx, job.ID, "result.txt", "text/plain", []byte("result"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE job_artifacts SET expires_at=now()-interval '1 second' WHERE id=$1`, item.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Cleanup(ctx); err == nil {
		t.Fatal("storage deletion failure was ignored")
	}
	var exists bool
	if err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM job_artifacts WHERE id=$1)`, item.ID).Scan(&exists); err != nil || !exists {
		t.Fatalf("metadata was lost: exists=%v err=%v", exists, err)
	}
	failing.Store(false)
	if err := store.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM job_artifacts WHERE id=$1)`, item.ID).Scan(&exists); err != nil || exists {
		t.Fatalf("metadata remained after successful retry: exists=%v err=%v", exists, err)
	}
}

func TestArtifactUploadDownloadDeletionAndExpiry(t *testing.T) {
	endpoint := os.Getenv("TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("TEST_S3_ENDPOINT is not set")
	}
	ctx, db := database(t)
	store := artifacts.Store{DB: db, Endpoint: endpoint, PublicEndpoint: endpoint, AccessKey: os.Getenv("TEST_S3_ACCESS_KEY"), SecretKey: os.Getenv("TEST_S3_SECRET_KEY"), Bucket: "forgequeue-integration", MaxBytes: 1024, Retention: time.Hour}
	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatal(err)
	}
	job, _, err := (jobs.Store{DB: db}).Submit(ctx, jobs.SubmitRequest{Type: "echo", Queue: unique("artifacts"), Payload: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, `DELETE FROM jobs WHERE id=$1`, job.ID)
	if _, err := store.Put(ctx, job.ID, "large.bin", "application/octet-stream", make([]byte, 1025)); err == nil {
		t.Fatal("oversized upload accepted")
	}
	item, err := store.Put(ctx, job.ID, "result.txt", "text/plain", []byte("artifact contents"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Delete(ctx, job.ID, item.ID)
	items, err := store.List(ctx, job.ID)
	if err != nil || len(items) != 1 {
		t.Fatalf("list items=%d err=%v", len(items), err)
	}
	url, err := store.DownloadURL(ctx, job.ID, item.ID, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || string(body) != "artifact contents" {
		t.Fatalf("download status=%d body=%q err=%v", response.StatusCode, body, err)
	}
	if _, err := store.DownloadURL(ctx, "00000000-0000-0000-0000-000000000000", item.ID, time.Minute); err == nil {
		t.Fatal("cross-job download accepted")
	}
	if _, err := db.Exec(ctx, `UPDATE job_artifacts SET expires_at=now()-interval '1 second' WHERE id=$1`, item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DownloadURL(ctx, job.ID, item.ID, time.Minute); err == nil {
		t.Fatal("expired download accepted")
	}
	if err := store.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	items, err = store.List(ctx, job.ID)
	if err != nil || len(items) != 0 {
		t.Fatalf("cleanup items=%d err=%v", len(items), err)
	}
}
