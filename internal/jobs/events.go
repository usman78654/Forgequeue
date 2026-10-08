package jobs

import (
	"context"
	"encoding/json"
	"time"
)

const MaxEventsPerJob = 1000

type Event struct {
	ID        int64           `json:"id"`
	JobID     string          `json:"job_id"`
	Attempt   int             `json:"attempt"`
	Type      string          `json:"type"`
	Level     string          `json:"level"`
	Message   string          `json:"message"`
	Progress  *float64        `json:"progress,omitempty"`
	Data      json.RawMessage `json:"data"`
	CreatedAt time.Time       `json:"created_at"`
}
type EventInput struct {
	Attempt              int
	Type, Level, Message string
	Progress             *float64
	Data                 json.RawMessage
}
type EventPage struct {
	Items     []Event `json:"items"`
	NextAfter int64   `json:"next_after,omitempty"`
	HasMore   bool    `json:"has_more"`
}

func (s Store) AppendEvent(ctx context.Context, jobID string, input EventInput) (Event, error) {
	if input.Type == "" {
		input.Type = "log"
	}
	if input.Level == "" {
		input.Level = "info"
	}
	if len(input.Data) == 0 {
		input.Data = json.RawMessage(`{}`)
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Event{}, err
	}
	defer tx.Rollback(ctx)
	var event Event
	err = tx.QueryRow(ctx, `INSERT INTO job_events(job_id,attempt,event_type,level,message,progress,data) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id,job_id,attempt,event_type,level,message,progress,data,created_at`, jobID, input.Attempt, input.Type, input.Level, input.Message, input.Progress, input.Data).Scan(&event.ID, &event.JobID, &event.Attempt, &event.Type, &event.Level, &event.Message, &event.Progress, &event.Data, &event.CreatedAt)
	if err != nil {
		return Event{}, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM job_events WHERE job_id=$1 AND id < COALESCE((SELECT id FROM job_events WHERE job_id=$1 ORDER BY id DESC OFFSET $2 LIMIT 1),0)`, jobID, MaxEventsPerJob-1); err != nil {
		return Event{}, err
	}
	return event, tx.Commit(ctx)
}
func (s Store) ListEvents(ctx context.Context, jobID string, after int64, limit int) (EventPage, error) {
	if limit < 1 || limit > 200 {
		limit = 100
	}
	rows, err := s.DB.Query(ctx, `SELECT id,job_id,attempt,event_type,level,message,progress,data,created_at FROM job_events WHERE job_id=$1 AND id>$2 ORDER BY id LIMIT $3`, jobID, after, limit+1)
	if err != nil {
		return EventPage{}, err
	}
	defer rows.Close()
	var items []Event
	for rows.Next() {
		var event Event
		if err = rows.Scan(&event.ID, &event.JobID, &event.Attempt, &event.Type, &event.Level, &event.Message, &event.Progress, &event.Data, &event.CreatedAt); err != nil {
			return EventPage{}, err
		}
		items = append(items, event)
	}
	if err = rows.Err(); err != nil {
		return EventPage{}, err
	}
	page := EventPage{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.NextAfter = items[limit-1].ID
		page.HasMore = true
	}
	return page, nil
}
