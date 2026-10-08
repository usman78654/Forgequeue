package workflows

import (
	"encoding/json"
	"time"
)

type StepRequest struct {
	Key             string               `json:"key"`
	Type            string               `json:"type"`
	Queue           string               `json:"queue,omitempty"`
	Payload         json.RawMessage      `json:"payload"`
	DependsOn       []string             `json:"depends_on,omitempty"`
	Priority        int16                `json:"priority,omitempty"`
	MaxAttempts     int                  `json:"max_attempts,omitempty"`
	TimeoutSeconds  int                  `json:"timeout_seconds,omitempty"`
	Condition       string               `json:"condition,omitempty"`
	ContinueOnError bool                 `json:"continue_on_error,omitempty"`
	FanOutFrom      string               `json:"fan_out_from,omitempty"`
	Compensation    *CompensationRequest `json:"compensation,omitempty"`
}
type CompensationRequest struct {
	Type    string          `json:"type"`
	Queue   string          `json:"queue,omitempty"`
	Payload json.RawMessage `json:"payload"`
}
type SubmitRequest struct {
	Name        string        `json:"name"`
	FailFast    *bool         `json:"fail_fast,omitempty"`
	Steps       []StepRequest `json:"steps"`
	OwnerID     string        `json:"-"`
	Traceparent string        `json:"-"`
}
type Step struct {
	Key             string          `json:"key"`
	Type            string          `json:"type"`
	Status          string          `json:"status"`
	DependsOn       []string        `json:"depends_on"`
	JobID           *string         `json:"job_id,omitempty"`
	Condition       string          `json:"condition,omitempty"`
	ContinueOnError bool            `json:"continue_on_error"`
	Output          json.RawMessage `json:"output,omitempty"`
	FanOutFrom      string          `json:"fan_out_from,omitempty"`
	ParentStepKey   string          `json:"parent_step_key,omitempty"`
	IsCompensation  bool            `json:"is_compensation"`
}
type Workflow struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	FailFast  bool      `json:"fail_fast"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Steps     []Step    `json:"steps"`
	OwnerID   *string   `json:"owner_id,omitempty"`
}

type ListFilter struct {
	Name, Status, Cursor        string
	OwnerID                     string
	Limit                       int
	CreatedAfter, CreatedBefore *time.Time
}
type Page struct {
	Items      []Workflow `json:"items"`
	NextCursor string     `json:"next_cursor,omitempty"`
}
