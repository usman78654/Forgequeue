# ForgeQueue Development Roadmap

Status legend: `PLANNED`, `IN PROGRESS`, `COMPLETE`, `BLOCKED`.

This file is updated as features are implemented and verified. A feature is only marked `COMPLETE` after tests or a runnable smoke check pass.

| # | Feature | Status | Completion criteria |
|---|---|---|---|
| 1 | Versioned database migrations | COMPLETE | Dedicated migration command, checksums, existing-database baseline, Compose migration gate, CI test |
| 2 | RabbitMQ reconnection supervisor | COMPLETE | Reconnect publisher and consumers, recreate channels, bounded backoff, readiness reflects broker state |
| 3 | Lease fencing tokens | COMPLETE | Stale workers cannot complete reclaimed jobs; concurrent recovery test passes |
| 4 | Running-job cancellation | COMPLETE | Cancellation signal reaches worker, cancellation reason is stored, workflow propagation works |
| 5 | Integration test suite | COMPLETE | Real PostgreSQL/RabbitMQ tests cover duplicates, crashes, retries, leases, workflows, and outbox recovery |
| 6 | Cursor pagination and filtering | COMPLETE | Jobs/workflows support stable cursors, queue/type/status/date filters, documented limits |
| 7 | Job events, progress, and logs | COMPLETE | Workers report lifecycle/progress/log events; 1,000-event retention, pagination, and resumable SSE are verified |
| 8 | Authentication and authorization | COMPLETE | SHA-256 API-key digests, admin/operator/viewer RBAC, ownership checks, key lifecycle, and audit trail are verified |
| 9 | React operations dashboard | COMPLETE | Submit, inspect, cancel, retry, schedule, and monitor jobs, workflows, workers, and schedules from the bundled console |
| 10 | Visual DAG builder | COMPLETE | React Flow editor builds dependencies, selects conditions, previews JSON, validates, and submits workflows |
| 11 | Advanced retry policies | COMPLETE | Fixed/linear/exponential policies, maximum delay, jitter, and non-retryable errors are implemented and tested |
| 12 | Workflow conditions and outputs | COMPLETE | Typed output references, success/failure/always conditions, and continue-on-error behavior pass integration tests |
| 13 | Dynamic fan-out and compensation | COMPLETE | Runtime fan-out/fan-in and saga compensation are durable and pass recovery-oriented integration tests |
| 14 | Scheduler leadership | COMPLETE | Dedicated scheduler uses a session advisory lock; metrics and two-replica failover are verified |
| 15 | Queue routing and worker capabilities | COMPLETE | Multi-queue capability routing, pause/resume, and atomic per-queue concurrency limits are verified |
| 16 | Rate limits and tenant quotas | COMPLETE | Per-principal submission, concurrency, payload, and schedule quotas enforce backpressure |
| 17 | OpenTelemetry tracing | COMPLETE | W3C context crosses API, PostgreSQL outbox, RabbitMQ, worker, and workflow-created jobs via OTLP |
| 18 | Prometheus alert rules | COMPLETE | Alerts cover queue growth, failures, DLQ, workers, leases, scheduler, and outbox age |
| 19 | Queue-depth autoscaling | COMPLETE | KEDA or Prometheus Adapter scales workers from RabbitMQ depth |
| 20 | Graceful worker draining | COMPLETE | Shutdown stops consumption, renews active leases, and safely requeues unfinished work |
| 21 | Object-backed logs and artifacts | COMPLETE | S3/MinIO storage with metadata, size limits, retention, and signed downloads |
| 22 | Security hardening | COMPLETE | TLS guidance, safe handler registry, secret management, network policy, and scans |
| 23 | Load and chaos testing | COMPLETE | Reproducible throughput report plus broker/worker/database failure scenarios |
| 24 | Portfolio demo and ADRs | COMPLETE | One-command demo and architecture decision records explain major tradeoffs |

## Already implemented

- Durable immediate and scheduled jobs
- RabbitMQ priorities and publisher confirms
- PostgreSQL state machine and transactional outbox
- Concurrent workers, retries, timeouts, leases, heartbeats, and crash recovery
- Idempotency, dead-letter state, requeue, attempts, and job results
- DAG validation, parallel branches, fan-in, and fail-fast workflows
- Recurring schedules
- REST/OpenAPI API and CLI
- Prometheus/Grafana monitoring
- Docker Compose, Kubernetes manifests, HPA, PDB, and CI

## Verification log

- `2026-09-29` — Migration runner passed upgrade-baseline and clean-database checks with three ordered checksummed migrations.
- `2026-09-29` — RabbitMQ stop/restart test returned readiness `503`, retained the job in PostgreSQL, recovered delivery, and completed it without restarting API or worker.
- `2026-09-29` — PostgreSQL fencing integration test reclaimed an expired lease, rejected the stale worker result, and committed only the current worker result.
- `2026-09-29` — Running job and workflow cancellation tests stopped active work, retained reasons, closed attempts, and cancelled dependent steps.
- `2026-09-29` — Distributed integration suite passed idempotency, duplicate claim, retry/DLQ, crash recovery, fencing, DAG release, and RabbitMQ outbox delivery scenarios.
- `2026-09-29` — Job and workflow pagination passed multi-page traversal with opaque cursors, no overlap, filters, and invalid-cursor rejection.
- `2026-09-29` — Job events passed PostgreSQL retention/paging tests and a live worker smoke test with intermediate progress, terminal state, and resumable SSE delivery.
- `2026-09-30` — Authentication returned 401 without a key and enforced viewer read-only access, resource ownership, admin key management, and mutation auditing.
- `2026-09-30` — The production React bundle loaded from `/app/`; the operations console and SVG DAG builder share the authenticated API client.
- `2026-09-30` — Retry policy unit tests and a live non-retryable job confirmed fixed/linear/exponential delays, caps, jitter control, and immediate dead-lettering.
- `2026-09-30` — PostgreSQL integration and live worker tests resolved step output references, ran failure branches, and completed continue-on-error workflows.
- `2026-10-01` — Dynamic fan-out created durable child steps, fan-in waited for all outputs, and a failed saga reached `compensated` after its recovery job.
- `2026-10-01` — Two scheduler replicas demonstrated PostgreSQL advisory-lock exclusion and four-second automatic leadership failover.
- `2026-10-01` — Queue pause/resume retained unpublished work, a concurrency cap held one of two jobs queued, and capability mismatches were rejected.
- `2026-10-01` — Tenant quota integration tests enforced rate, active-job, payload, and schedule boundaries with API backpressure.
- `2026-10-01` — A fixed W3C trace ID was preserved through job/outbox state and produced API, outbox, and worker spans in the OTLP collector.
- `2026-10-04` — Added seven Prometheus alerts, scheduler scraping, queue/lease/outbox-age metrics, and RabbitMQ queue-depth scaling with KEDA.
- `2026-10-04` — Added bounded worker draining, private MinIO/S3 artifacts, security hardening, load/chaos tooling, a one-command demo, and six ADRs.
