package jobs

import (
	"encoding/json"
	"time"
)

type Job struct {
	ID                      string          `json:"id"`
	Type                    string          `json:"type"`
	Queue                   string          `json:"queue"`
	Payload                 json.RawMessage `json:"payload"`
	Status                  string          `json:"status"`
	Priority                int16           `json:"priority"`
	Attempt                 int             `json:"attempt"`
	LeaseToken              int64           `json:"lease_token,omitempty"`
	MaxAttempts             int             `json:"max_attempts"`
	TimeoutSeconds          int             `json:"timeout_seconds"`
	ScheduledAt             time.Time       `json:"scheduled_at"`
	LastError               *string         `json:"last_error,omitempty"`
	Result                  json.RawMessage `json:"result,omitempty"`
	CreatedAt               time.Time       `json:"created_at"`
	UpdatedAt               time.Time       `json:"updated_at"`
	CancellationRequestedAt *time.Time      `json:"cancellation_requested_at,omitempty"`
	CancellationReason      *string         `json:"cancellation_reason,omitempty"`
	OwnerID                 *string         `json:"owner_id,omitempty"`
	RetryPolicy             string          `json:"retry_policy"`
	RetryInitialSeconds     int             `json:"retry_initial_seconds"`
	RetryMaxSeconds         int             `json:"retry_max_seconds"`
	RetryJitter             bool            `json:"retry_jitter"`
	NonRetryableErrors      []string        `json:"non_retryable_errors,omitempty"`
}

type CancelRequest struct {
	Reason string `json:"reason,omitempty"`
}

type ListFilter struct {
	Status, Queue, Type, Cursor string
	OwnerID                     string
	Limit                       int
	CreatedAfter, CreatedBefore *time.Time
}
type Page struct {
	Items      []Job  `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}

type Attempt struct {
	Attempt    int        `json:"attempt"`
	WorkerID   string     `json:"worker_id"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Error      *string    `json:"error,omitempty"`
}

type SubmitRequest struct {
	Type                 string          `json:"type"`
	Queue                string          `json:"queue,omitempty"`
	Payload              json.RawMessage `json:"payload"`
	Priority             int16           `json:"priority,omitempty"`
	MaxAttempts          int             `json:"max_attempts,omitempty"`
	TimeoutSeconds       int             `json:"timeout_seconds,omitempty"`
	ScheduledAt          *time.Time      `json:"scheduled_at,omitempty"`
	IdempotencyKey       string          `json:"idempotency_key,omitempty"`
	OwnerID              string          `json:"-"`
	RetryPolicy          string          `json:"retry_policy,omitempty"`
	RetryInitialSeconds  int             `json:"retry_initial_seconds,omitempty"`
	RetryMaxSeconds      int             `json:"retry_max_seconds,omitempty"`
	RetryJitter          *bool           `json:"retry_jitter,omitempty"`
	NonRetryableErrors   []string        `json:"non_retryable_errors,omitempty"`
	RequiredCapabilities []string        `json:"required_capabilities,omitempty"`
	Traceparent          string          `json:"-"`
}
