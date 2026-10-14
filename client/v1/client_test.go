package v1

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPaginationAndAuthentication(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" || r.URL.Query().Get("cursor") != "a+b" || r.URL.Query().Get("queue") != "jobs.default" {
			t.Error("missing authentication or escaped filters")
		}
		w.Write([]byte(`{"items":[{"id":"job-1"}],"next_cursor":"next"}`))
	}))
	defer srv.Close()
	c, err := New(srv.URL, "secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	page, err := c.Jobs(context.Background(), JobFilter{Cursor: "a+b", Queue: "jobs.default"})
	if err != nil || len(page.Items) != 1 || page.NextCursor != "next" {
		t.Fatalf("page=%+v err=%v", page, err)
	}
}

func TestWritesNeverRetryAndTypedErrors(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(503)
		w.Write([]byte(`{"title":"unavailable","detail":"retry later"}`))
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "secret", nil)
	_, err := c.Submit(context.Background(), SubmitJob{Type: "echo", Payload: map[string]string{"message": "hi"}})
	var apiErr *APIError
	if calls != 1 || !errors.As(err, &apiErr) || apiErr.StatusCode != 503 || apiErr.Detail != "retry later" {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestReadRetriesAndCancellation(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(503)
			return
		}
		w.Write([]byte(`{"id":"job-1"}`))
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "secret", nil)
	if _, err := c.Job(context.Background(), "job-1"); err != nil || calls != 3 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Job(ctx, "job-1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation: %v", err)
	}
}

func TestRejectsCredentialRedirectAndExternalPaths(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/other", 302) }))
	defer srv.Close()
	c, _ := New(srv.URL, "secret", nil)
	if err := c.Do(context.Background(), "GET", "https://other.example/v1/jobs", nil, nil); err == nil {
		t.Fatal("accepted absolute URL")
	}
	var apiErr *APIError
	if err := c.Do(context.Background(), "GET", "/v1/jobs", nil, nil); !errors.As(err, &apiErr) || apiErr.StatusCode != 302 {
		t.Fatalf("followed redirect: %v", err)
	}
	if _, err := New("file:///tmp", "secret", nil); err == nil {
		t.Fatal("accepted invalid base")
	}
}
