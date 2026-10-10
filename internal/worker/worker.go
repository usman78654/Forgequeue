package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	amqp "github.com/rabbitmq/amqp091-go"
	handlersdk "github.com/usman78654/Forgequeue/handler"
	"github.com/usman78654/Forgequeue/internal/broker"
	"github.com/usman78654/Forgequeue/internal/jobs"
	"github.com/usman78654/Forgequeue/internal/tracing"
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
	Handlers             *handlersdk.Registry
}

func (w Worker) Run(ctx context.Context) error {
	if w.Handlers == nil {
		w.Handlers = defaultHandlers(w.AllowCommand)
	}
	queues := w.Queues
	if len(queues) == 0 {
		queues = []string{w.Queue}
	}
	consumeCtx, stopConsuming := context.WithCancel(context.Background())
	executionCtx, stopExecutions := context.WithCancel(context.Background())
	defer stopConsuming()
	defer stopExecutions()
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
	if w.Draining != nil {
		w.Draining.Store(true)
	}
	stopHeartbeat()
	drainCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	_ = w.Store.SetWorkerState(drainCtx, w.ID, "draining")
	cancel()
	stopConsuming()
	done := make(chan struct{})
	go func() { loops.Wait(); close(done) }()
	timeout := w.DrainTimeout
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	select {
	case <-done:
	case <-time.After(timeout):
		stopExecutions()
		releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if n, err := w.Store.ReleaseWorkerJobs(releaseCtx, w.ID); err != nil {
			slog.Error("release jobs during drain", "error", err)
		} else {
			slog.Warn("drain timeout; jobs released", "count", n)
		}
		releaseCancel()
	}
	offlineCtx, offlineCancel := context.WithTimeout(context.Background(), 3*time.Second)
	_ = w.Store.SetWorkerState(offlineCtx, w.ID, "offline")
	offlineCancel()
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
		case <-ctx.Done():
			return
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
	reporter := eventReporter{report: report}
	result, e := w.Handlers.Execute(runCtx, handlersdk.Job{ID: j.ID, Type: j.Type, Queue: j.Queue, Attempt: j.Attempt, Payload: j.Payload}, reporter)
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

type eventReporter struct{ report func(jobs.EventInput) }

func (r eventReporter) Progress(value float64, message string) {
	if value < 0 {
		value = 0
	}
	if value > 1 {
		value = 1
	}
	r.report(jobs.EventInput{Type: "progress", Message: message, Progress: &value})
}

func (r eventReporter) Log(level, message string, data json.RawMessage) {
	r.report(jobs.EventInput{Type: "log", Level: level, Message: message, Data: data})
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
func progress(value float64) *float64 { return &value }
