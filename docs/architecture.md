# Architecture

ForgeQueue uses PostgreSQL as the source of truth and RabbitMQ as the delivery signal.

```text
client -> REST API -> PostgreSQL jobs + outbox (one transaction)
                         |                 |
                         |            outbox relay
                         |                 v
                         +<----------- RabbitMQ -> workers

Prometheus <- /metrics                          Grafana <- Prometheus
```

## Delivery and recovery model

- Delivery is **at least once**. Workers atomically move a queued job to running before executing it.
- A repeated delivery cannot execute a job that is no longer queued.
- A lease protects work owned by a worker. If that worker crashes, the API recovery loop changes the expired job to retrying; the scheduler republishes it.
- Submission and its outbox entry commit together. The relay may publish twice if it crashes between publish and marking the row, which is safe because claiming is guarded by job state.
- An idempotency key gives clients exactly-one logical submission within the retained job history.
- Failed attempts use exponential backoff with jitter. Exhausted jobs enter the `dead` state for inspection.

The first milestone supports built-in `echo`, `sleep`, and `command` handlers. Running arbitrary commands is intended for isolated demo workers only; production deployments should register trusted typed handlers instead.

## Job state machine

```text
scheduled -> queued -> running -> succeeded
                ^         |
                |         +-> retrying -> queued
                |         +-> dead
                +---------------- (expired lease)

scheduled/queued/retrying -> cancelled
running -> cancelling -> cancelled
```

## Next milestones

1. Queue routing, rate limits, and per-tenant quotas.
2. Conditional and compensation steps for saga-style workflows.
3. OpenTelemetry traces and alert rules.
4. Queue-depth autoscaling through KEDA or Prometheus Adapter.
5. Go SDK, crash-injection integration tests, and load tests.
