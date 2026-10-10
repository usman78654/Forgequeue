package handler

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
)

type recordingReporter struct {
	mu       sync.Mutex
	progress []float64
	logs     []string
}

func (r *recordingReporter) Progress(value float64, _ string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.progress = append(r.progress, value)
}
func (r *recordingReporter) Log(_, message string, _ json.RawMessage) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logs = append(r.logs, message)
}

func TestTypedHandler(t *testing.T) {
	type input struct {
		Name string `json:"name"`
	}
	type output struct {
		Greeting string `json:"greeting"`
	}
	r := NewRegistry()
	if err := RegisterJSON(r, "greet", func(_ context.Context, in input, report Reporter) (output, error) {
		report.Progress(.5, "greeting prepared")
		report.Log("info", "greeting complete", nil)
		return output{Greeting: "hello " + in.Name}, nil
	}); err != nil {
		t.Fatal(err)
	}
	reporter := &recordingReporter{}
	result, err := r.Execute(context.Background(), Job{Type: "greet", Payload: json.RawMessage(`{"name":"Ada"}`)}, reporter)
	if err != nil {
		t.Fatal(err)
	}
	if string(result) != `{"greeting":"hello Ada"}` {
		t.Fatalf("result=%s", result)
	}
	if len(reporter.progress) != 1 || len(reporter.logs) != 1 {
		t.Fatalf("progress=%v logs=%v", reporter.progress, reporter.logs)
	}
}

func TestTypedHandlerRejectsInvalidPayloadPermanently(t *testing.T) {
	type input struct {
		Name string `json:"name"`
	}
	r := NewRegistry()
	if err := RegisterJSON(r, "greet", func(context.Context, input, Reporter) (struct{}, error) { return struct{}{}, nil }); err != nil {
		t.Fatal(err)
	}
	_, err := r.Execute(context.Background(), Job{Type: "greet", Payload: json.RawMessage(`{"unknown":true}`)}, nil)
	if !IsPermanent(err) {
		t.Fatalf("expected permanent decoding error, got %v", err)
	}
}

func TestRegistryRejectsDuplicatesAndUnknownTypes(t *testing.T) {
	r := NewRegistry()
	h := Func(func(context.Context, Job, Reporter) (json.RawMessage, error) { return nil, nil })
	if err := r.Register("job", h); err != nil {
		t.Fatal(err)
	}
	if err := r.Register("job", h); err == nil {
		t.Fatal("expected duplicate registration error")
	}
	_, err := r.Execute(context.Background(), Job{Type: "missing"}, nil)
	if !IsPermanent(err) {
		t.Fatalf("unknown handler error should be permanent: %v", err)
	}
}

func TestRegistryContainsPanics(t *testing.T) {
	r := NewRegistry()
	r.MustRegister("panic", Func(func(context.Context, Job, Reporter) (json.RawMessage, error) {
		panic("boom")
	}))
	_, err := r.Execute(context.Background(), Job{Type: "panic"}, nil)
	if err == nil || errors.Is(err, context.Canceled) {
		t.Fatalf("expected contained panic, got %v", err)
	}
}

func TestPermanentPreservesCause(t *testing.T) {
	cause := errors.New("invalid account")
	err := Permanent(cause)
	if !IsPermanent(err) || !errors.Is(err, cause) || Permanent(err) != err {
		t.Fatalf("permanent error did not preserve cause: %v", err)
	}
}
