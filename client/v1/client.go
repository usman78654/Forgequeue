// Package v1 provides a context-aware client for the ForgeQueue v1 HTTP API.
package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxResponseBytes = 16 << 20

type Client struct {
	base string
	key  string
	http *http.Client
}

// New validates the API origin and installs a timeout and redirect protection.
// Use HTTPS outside local development. The key is never included in errors.
func New(baseURL, apiKey string, transport http.RoundTripper) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("invalid ForgeQueue API base URL")
	}
	if strings.TrimSpace(apiKey) == "" || strings.ContainsAny(apiKey, "\r\n") {
		return nil, fmt.Errorf("an API key is required")
	}
	return &Client{base: strings.TrimRight(baseURL, "/"), key: apiKey, http: &http.Client{
		Timeout: 30 * time.Second, Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

type APIError struct {
	StatusCode int    `json:"-"`
	Title      string `json:"title"`
	Detail     string `json:"detail"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("ForgeQueue API returned %d: %s", e.StatusCode, e.Title)
}

// Do executes a v1 endpoint. Reads retry 502/503/504 at most twice; writes never
// retry automatically. Cancellation interrupts requests and retry backoff.
// out may be nil for operations without a response body.
func (c *Client) Do(ctx context.Context, method, path string, body, out any) error {
	u, err := url.Parse(path)
	if err != nil || u.IsAbs() || u.Host != "" || !strings.HasPrefix(u.Path, "/v1/") || u.Fragment != "" || strings.Contains(u.Path, "..") {
		return fmt.Errorf("path must be a relative v1 API endpoint")
	}
	var payload []byte
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		res, err := c.http.Do(req)
		if err != nil {
			return err
		}
		data, readErr := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
		res.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read response: %w", readErr)
		}
		if len(data) > maxResponseBytes {
			return fmt.Errorf("API response exceeds 16 MiB limit")
		}
		if method == http.MethodGet && attempt < 2 && (res.StatusCode == 502 || res.StatusCode == 503 || res.StatusCode == 504) {
			timer := time.NewTimer(time.Duration(attempt+1) * 150 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			continue
		}
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			apiErr := &APIError{StatusCode: res.StatusCode}
			_ = json.Unmarshal(data, apiErr)
			if apiErr.Title == "" {
				apiErr.Title = http.StatusText(res.StatusCode)
			}
			return apiErr
		}
		if out == nil || res.StatusCode == http.StatusNoContent {
			return nil
		}
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
		return nil
	}
}

type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}

type Job struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	Queue       string          `json:"queue"`
	Status      string          `json:"status"`
	Payload     json.RawMessage `json:"payload"`
	Result      json.RawMessage `json:"result,omitempty"`
	Attempt     int             `json:"attempt"`
	MaxAttempts int             `json:"max_attempts"`
	LastError   *string         `json:"last_error,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}

type SubmitJob struct {
	Type           string `json:"type"`
	Queue          string `json:"queue,omitempty"`
	Payload        any    `json:"payload"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
	MaxAttempts    int    `json:"max_attempts,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
}

type JobFilter struct {
	Status, Queue, Type, Cursor string
	Limit                       int
}

func (c *Client) Jobs(ctx context.Context, filter JobFilter) (Page[Job], error) {
	q := url.Values{}
	for key, value := range map[string]string{"status": filter.Status, "queue": filter.Queue, "type": filter.Type, "cursor": filter.Cursor} {
		if value != "" {
			q.Set(key, value)
		}
	}
	if filter.Limit > 0 {
		q.Set("limit", fmt.Sprint(filter.Limit))
	}
	var page Page[Job]
	err := c.Do(ctx, http.MethodGet, "/v1/jobs?"+q.Encode(), nil, &page)
	return page, err
}

func (c *Client) Job(ctx context.Context, id string) (Job, error) {
	var job Job
	err := c.Do(ctx, http.MethodGet, "/v1/jobs/"+url.PathEscape(id), nil, &job)
	return job, err
}

func (c *Client) Submit(ctx context.Context, input SubmitJob) (Job, error) {
	var job Job
	err := c.Do(ctx, http.MethodPost, "/v1/jobs", input, &job)
	return job, err
}

func (c *Client) Cancel(ctx context.Context, id, reason string) error {
	return c.Do(ctx, http.MethodPost, "/v1/jobs/"+url.PathEscape(id)+"/cancel", map[string]string{"reason": reason}, nil)
}

func (c *Client) Requeue(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodPost, "/v1/jobs/"+url.PathEscape(id)+"/requeue", nil, nil)
}
