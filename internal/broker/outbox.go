package broker

import (
	"context"
	"github.com/usman78654/Forgequeue/internal/tracing"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"log/slog"
	"time"
)

func RunOutbox(ctx context.Context, db *pgxpool.Pool, r *Rabbit, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := publishBatch(ctx, db, r); err != nil {
				if ctx.Err() != nil {
					return
				}
				slog.Error("outbox publish", "error", err)
			}
		}
	}
}
func publishBatch(ctx context.Context, db *pgxpool.Pool, r *Rabbit) error {
	rows, e := db.Query(ctx, `SELECT o.id,o.job_id,o.queue,o.priority,COALESCE(o.traceparent,'') FROM outbox o WHERE o.published_at IS NULL AND NOT EXISTS(SELECT 1 FROM queue_controls q WHERE q.queue=o.queue AND q.paused) ORDER BY o.id LIMIT 100`)
	if e != nil {
		return e
	}
	defer rows.Close()
	type item struct {
		id          int64
		job, queue  string
		p           int16
		traceparent string
	}
	var xs []item
	for rows.Next() {
		var x item
		if e = rows.Scan(&x.id, &x.job, &x.queue, &x.p, &x.traceparent); e != nil {
			return e
		}
		xs = append(xs, x)
	}
	for _, x := range xs {
		spanCtx, span := otel.Tracer("forgequeue/outbox").Start(tracing.Context(x.traceparent), "outbox.publish", trace.WithAttributes(attribute.String("job.id", x.job), attribute.String("messaging.destination", x.queue)))
		if e = r.Declare(ctx, x.queue); e != nil {
			return e
		}
		if e = r.Publish(ctx, x.queue, x.job, x.p, tracing.Traceparent(spanCtx)); e != nil {
			span.RecordError(e)
			span.End()
			return e
		}
		if _, e = db.Exec(ctx, `UPDATE outbox SET published_at=now() WHERE id=$1`, x.id); e != nil {
			span.RecordError(e)
			span.End()
			return e
		}
		span.End()
	}
	return nil
}
