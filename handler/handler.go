// Package handler provides the public worker handler API for ForgeQueue.
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
)

// Job is the stable execution metadata exposed to application handlers.
type Job struct {
	ID      string
	Type    string
	Queue   string
	Attempt int
	Payload json.RawMessage
}

// Reporter records durable progress and structured log events for a job.
type Reporter interface {
	Progress(value float64, message string)
	Log(level, message string, data json.RawMessage)
}

// Handler executes one job and returns a JSON result.
type Handler interface {
	Handle(context.Context, Job, Reporter) (json.RawMessage, error)
}

// Func adapts a function to Handler.
type Func func(context.Context, Job, Reporter) (json.RawMessage, error)

func (f Func) Handle(ctx context.Context, job Job, reporter Reporter) (json.RawMessage, error) {
	return f(ctx, job, reporter)
}

// Registry is safe for concurrent registration, lookup, and execution.
type Registry struct {
	mu       sync.RWMutex
	handlers map[string]Handler
}

func NewRegistry() *Registry { return &Registry{handlers: make(map[string]Handler)} }

func (r *Registry) Register(jobType string, h Handler) error {
	jobType = strings.TrimSpace(jobType)
	if jobType == "" {
		return errors.New("handler job type is required")
	}
	if h == nil {
		return fmt.Errorf("handler for %q is nil", jobType)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.handlers[jobType]; exists {
		return fmt.Errorf("handler for %q is already registered", jobType)
	}
	r.handlers[jobType] = h
	return nil
}

func (r *Registry) MustRegister(jobType string, h Handler) {
	if err := r.Register(jobType, h); err != nil {
		panic(err)
	}
}

func (r *Registry) Types() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	types := make([]string, 0, len(r.handlers))
	for jobType := range r.handlers {
		types = append(types, jobType)
	}
	sort.Strings(types)
	return types
}

func (r *Registry) Execute(ctx context.Context, job Job, reporter Reporter) (result json.RawMessage, err error) {
	r.mu.RLock()
	h, ok := r.handlers[job.Type]
	r.mu.RUnlock()
	if !ok {
		return nil, Permanent(fmt.Errorf("no handler registered for job type %q", job.Type))
	}
	if reporter == nil {
		reporter = noopReporter{}
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			result = nil
			err = fmt.Errorf("handler %q panicked: %v", job.Type, recovered)
		}
	}()
	return h.Handle(ctx, job, reporter)
}

// RegisterJSON registers a strongly typed handler. Invalid input and result
// encoding failures are permanent because retrying cannot change either value.
func RegisterJSON[Input, Output any](r *Registry, jobType string, fn func(context.Context, Input, Reporter) (Output, error)) error {
	if fn == nil {
		return errors.New("typed handler function is nil")
	}
	return r.Register(jobType, Func(func(ctx context.Context, job Job, reporter Reporter) (json.RawMessage, error) {
		var input Input
		decoder := json.NewDecoder(bytes.NewReader(job.Payload))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			return nil, Permanent(fmt.Errorf("decode %s payload: %w", job.Type, err))
		}
		if err := ensureEOF(decoder); err != nil {
			return nil, Permanent(fmt.Errorf("decode %s payload: %w", job.Type, err))
		}
		output, err := fn(ctx, input, reporter)
		if err != nil {
			return nil, err
		}
		result, err := json.Marshal(output)
		if err != nil {
			return nil, Permanent(fmt.Errorf("encode %s result: %w", job.Type, err))
		}
		return result, nil
	}))
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("payload contains multiple JSON values")
		}
		return err
	}
	return nil
}

type permanentError struct{ cause error }

func (permanentError) Permanent() bool { return true }
func (e permanentError) Error() string { return e.cause.Error() }
func (e permanentError) Unwrap() error { return e.cause }

// Permanent marks an error as non-retryable. The original error remains
// available through errors.Is and errors.As.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	var marked interface{ Permanent() bool }
	if errors.As(err, &marked) && marked.Permanent() {
		return err
	}
	return permanentError{cause: err}
}

func IsPermanent(err error) bool {
	var marked interface{ Permanent() bool }
	return errors.As(err, &marked) && marked.Permanent()
}

type noopReporter struct{}

func (noopReporter) Progress(float64, string)            {}
func (noopReporter) Log(string, string, json.RawMessage) {}
