package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/usman78654/Forgequeue/internal/broker"
	"github.com/usman78654/Forgequeue/internal/jobs"
	"github.com/usman78654/Forgequeue/internal/tracing"
	"github.com/prometheus/client_golang/prometheus"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var processed = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "forgequeue_jobs_processed_total", Help: "Processed jobs."}, []string{"status", "type"})
var duration = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "forgequeue_job_duration_seconds", Help: "Job execution duration.", Buckets: prometheus.DefBuckets}, []string{"type", "status"})

func init() { prometheus.MustRegister(processed, duration) }

type Worker struct {
	Store                jobs.Store
	Rabbit               *broker.Rabbit
	ID, Queue            string
	Queues, Capabilities []string
	Concurrency          int
	Lease                time.Duration
	DrainTimeout         time.Duration
	AllowCommand         bool
	Draining             *atomic.Bool
}

func (w Worker) Run(ctx context.Context) error {
	queues := w.Queues
	if len(queues) == 0 {
		queues = []string{w.Queue}
	}
	consumeCtx, stopConsuming := context.WithCancel(context.Background())
	executionCtx, stopExecutions := context.WithCancel(context.Background())
	defer stopConsuming(); defer stopExecutions()
	deliveries := make(chan amqp.Delivery)
	var forwarders sync.WaitGroup
	for i, queue := range queues {
		source := w.Rabbit.Consume(consumeCtx, queue, fmt.Sprintf("%s-%d", w.ID, i), w.Concurrency)
		forwarders.Add(1)
		go func() {
			defer forwarders.Done()
			for d := range source {
				select {
				case deliveries <- d:
				case <-consumeCtx.Done():
					return
				}
			}
		}()
	}
	go func() { forwarders.Wait(); close(deliveries) }()
	heartbeatCtx, stopHeartbeat := context.WithCancel(context.Background())
	defer stopHeartbeat()
	go w.heartbeat(heartbeatCtx)
	var loops sync.WaitGroup
	for i := 0; i < w.Concurrency; i++ {
		loops.Add(1)
		go func() { defer loops.Done(); w.loop(executionCtx, deliveries) }()
	}
	<-ctx.Done()
	if w.Draining != nil { w.Draining.Store(true) }
	stopHeartbeat()
	drainCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	_ = w.Store.SetWorkerState(drainCtx, w.ID, "draining")
	cancel()
	stopConsuming()
	done := make(chan struct{})
	go func() { loops.Wait(); close(done) }()
	timeout := w.DrainTimeout; if timeout <= 0 { timeout = 45*time.Second }
	select {
	case <-done:
	case <-time.After(timeout):
		stopExecutions()
		releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if n, err := w.Store.ReleaseWorkerJobs(releaseCtx, w.ID); err != nil { slog.Error("release jobs during drain", "error", err) } else { slog.Warn("drain timeout; jobs released", "count", n) }
		releaseCancel()
	}
	offlineCtx, offlineCancel := context.WithTimeout(context.Background(), 3*time.Second)
	_ = w.Store.SetWorkerState(offlineCtx, w.ID, "offline"); offlineCancel()
	return ctx.Err()
}
func (w Worker) heartbeat(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		queues := w.Queues
		if len(queues) == 0 {
			queues = []string{w.Queue}
		}
		_ = w.Store.HeartbeatWorker(ctx, w.ID, queues, w.Capabilities, w.Concurrency)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (w Worker) loop(ctx context.Context, in <-chan amqp.Delivery) {
	for {
		select {
		case d, ok := <-in:
			if !ok {
				return
			}
			w.handle(ctx, d)
		case <-ctx.Done(): return
		}
	}
}
func (w Worker) handle(ctx context.Context, d amqp.Delivery) {
	var m broker.Message
	if json.Unmarshal(d.Body, &m) != nil {
		_ = d.Reject(false)
		return
	}
	ctx, span := otel.Tracer("forgequeue/worker").Start(tracing.Context(m.Traceparent), "job.execute", trace.WithAttributes(attribute.String("job.id", m.JobID), attribute.String("messaging.system", "rabbitmq")))
	defer span.End()
	j, e := w.Store.Claim(ctx, m.JobID, w.ID, w.Lease, w.Capabilities...)
	if errors.Is(e, jobs.ErrRoutingBlocked) {
		_ = d.Nack(false, true)
		time.Sleep(250 * time.Millisecond)
		return
	}
	if errors.Is(e, jobs.ErrNotFound) {
		_ = d.Ack(false)
		return
	}
	if e != nil {
		slog.Error("claim job", "error", e)
		_ = d.Nack(false, true)
		return
	}
	report := func(input jobs.EventInput) {
		input.Attempt = j.Attempt
		if _, err := w.Store.AppendEvent(ctx, j.ID, input); err != nil {
			slog.Warn("job event write failed", "job_id", j.ID, "error", err)
		}
	}
	report(jobs.EventInput{Type: "state", Message: "job started", Data: json.RawMessage(fmt.Sprintf(`{"worker_id":%q}`, w.ID))})
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(j.TimeoutSeconds)*time.Second)
	started := time.Now()
	maintenanceDone := make(chan bool, 1)
	go w.maintain(runCtx, j, cancel, maintenanceDone)
	result, e := execute(runCtx, j, report, w.AllowCommand)
	cancel()
	cancelled := <-maintenanceDone
	if cancelled {
		_, reason, _ := w.Store.CancellationRequested(ctx, j.ID, j.LeaseToken)
		e = w.Store.CompleteCancellation(ctx, j, reason)
		processed.WithLabelValues("cancelled", j.Type).Inc()
		duration.WithLabelValues(j.Type, "cancelled").Observe(time.Since(started).Seconds())
		report(jobs.EventInput{Type: "state", Level: "warn", Message: "job cancelled"})
	} else if e == nil {
		e = w.Store.Succeed(ctx, j, result)
		if errors.Is(e, jobs.ErrLeaseLost) {
			if requested, reason, _ := w.Store.CancellationRequested(ctx, j.ID, j.LeaseToken); requested {
				e = w.Store.CompleteCancellation(ctx, j, reason)
			}
		}
		processed.WithLabelValues("succeeded", j.Type).Inc()
		duration.WithLabelValues(j.Type, "succeeded").Observe(time.Since(started).Seconds())
		report(jobs.EventInput{Type: "state", Message: "job succeeded", Progress: progress(1)})
	} else {
		if persistErr := w.Store.Fail(ctx, j, e); persistErr != nil {
			e = fmt.Errorf("execution failed: %v; persisting failure: %w", e, persistErr)
		}
		processed.WithLabelValues("failed", j.Type).Inc()
		duration.WithLabelValues(j.Type, "failed").Observe(time.Since(started).Seconds())
		report(jobs.EventInput{Type: "state", Level: "error", Message: e.Error()})
	}
	if e != nil {
		slog.Error("job processing", "job_id", j.ID, "error", e)
	}
	_ = d.Ack(false)
}
func (w Worker) maintain(ctx context.Context, job jobs.Job, cancel context.CancelFunc, done chan<- bool) {
	cancelled := false
	defer func() { done <- cancelled; close(done) }()
	interval := w.Lease / 3
	if interval < time.Second {
		interval = time.Second
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	lastRenewal := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			requested, _, err := w.Store.CancellationRequested(ctx, job.ID, job.LeaseToken)
			if err != nil {
				slog.Warn("cancellation poll failed", "job_id", job.ID, "error", err)
				return
			}
			if requested {
				cancelled = true
				cancel()
				return
			}
			if time.Since(lastRenewal) >= interval {
				if err = w.Store.RenewLease(ctx, job.ID, w.ID, job.LeaseToken, w.Lease); err != nil {
					slog.Warn("lease renewal failed", "job_id", job.ID, "error", err)
					return
				}
				lastRenewal = time.Now()
			}
		}
	}
}
func execute(ctx context.Context, j jobs.Job, report func(jobs.EventInput), allowCommand bool) ([]byte, error) {
	switch j.Type {
	case "echo":
		report(jobs.EventInput{Type: "log", Message: "echo payload processed"})
		return j.Payload, nil
	case "sleep":
		var p struct {
			Duration string `json:"duration"`
		}
		if e := json.Unmarshal(j.Payload, &p); e != nil {
			return nil, e
		}
		d, e := time.ParseDuration(p.Duration)
		if e != nil {
			return nil, e
		}
		if d <= 0 {
			return []byte(`{"slept":true}`), nil
		}
		interval := d / 10
		if interval < 250*time.Millisecond {
			interval = 250 * time.Millisecond
		}
		if interval > 2*time.Second {
			interval = 2 * time.Second
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		timer := time.NewTimer(d)
		defer timer.Stop()
		started := time.Now()
		for {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-ticker.C:
				value := time.Since(started).Seconds() / d.Seconds()
				if value > .99 {
					value = .99
				}
				report(jobs.EventInput{Type: "progress", Message: "sleep progress", Progress: &value})
			case <-timer.C:
				return []byte(`{"slept":true}`), nil
			}
		}
	case "command":
		if !allowCommand { return nil, errors.New("command handler is disabled; use a registered safe handler or explicitly set ALLOW_COMMAND_HANDLER=true") }
		var p struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		}
		if e := json.Unmarshal(j.Payload, &p); e != nil {
			return nil, e
		}
		if p.Command == "" {
			return nil, errors.New("command is required")
		}
		out, e := exec.CommandContext(ctx, p.Command, p.Args...).CombinedOutput()
		if e != nil {
			report(jobs.EventInput{Type: "log", Level: "error", Message: "command failed", Data: logData(out)})
			return nil, fmt.Errorf("%w: %s", e, out)
		}
		report(jobs.EventInput{Type: "log", Message: "command completed", Data: logData(out)})
		b, _ := json.Marshal(map[string]string{"output": string(out)})
		return b, nil
	default:
		return nil, fmt.Errorf("unknown job type %q", j.Type)
	}
}
func progress(value float64) *float64 { return &value }
func logData(output []byte) json.RawMessage {
	const max = 16 * 1024
	if len(output) > max {
		output = output[:max]
	}
	body, _ := json.Marshal(map[string]string{"output": string(output)})
	return body
}
