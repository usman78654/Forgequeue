# ForgeQueue

A distributed job queue and workflow engine built with Go, PostgreSQL, and RabbitMQ, with an embedded TypeScript operations console, a typed Go worker SDK, and durable workflow orchestration.

## Included now

- Immediate and scheduled submission through a REST API
- Priority queues (0–9), retries with exponential backoff and jitter, execution timeouts
- Idempotency keys and duplicate-delivery protection
- PostgreSQL state machine, attempt history, leases, crash recovery, and dead-letter state
- Transactional outbox for reliable PostgreSQL-to-RabbitMQ dispatch
- Concurrent workers and graceful process shutdown
- DAG workflows with sequential/parallel steps, fan-in, and fail-fast cancellation
- Recurring interval schedules and delayed jobs
- Worker registration, heartbeats, and renewable execution leases
- Job listing, attempt history, and dead-letter requeue operations
- Durable job state, progress, and log events with bounded retention and SSE streaming
- Hashed API keys, viewer/operator/admin RBAC, per-owner resources, and mutation audit history
- Fixed, linear, or exponential retries with caps, optional jitter, and non-retryable error patterns
- Workflow output references, conditions, and continue-on-error steps
- Runtime fan-out/fan-in and saga compensation jobs
- Dedicated leader-elected scheduler with automatic failover
- Multi-queue workers, capability routing, queue controls, and concurrency caps
- Per-principal submission, concurrency, payload, and schedule quotas
- OpenTelemetry tracing through API, outbox, RabbitMQ, workers, and workflows
- Embedded React operations console and React Flow DAG builder
- JSON logs, health/readiness endpoints, Prometheus metrics, and provisioned Grafana
- OpenAPI contract, CLI, GitHub Actions CI, and Kubernetes scaling manifests

See [architecture](docs/architecture.md) for reliability semantics, [Go client](docs/go-client.md) for API integration, and [implementation checklist](implementation-plan.md) for completed and remaining improvements.

For a real offline DAG demo, see [sales report workflows](examples/report-worker/README.md): parallel dataset loading, validated joins, integer-cent totals, a generated Markdown file, and a permanent-error example that blocks downstream work.

## Run

Native builds require Go **1.27.2** or newer. CI and Docker use the patched baseline recorded in go.mod. See [CI security remediation](docs/ci-remediation.md) for dependency updates and verified checks.

```sh
docker compose up --build
```

Endpoints:

- API: `http://localhost:18080` (override with `API_PORT`)
- React console: `http://localhost:18080/app/`
- RabbitMQ UI: `http://localhost:15672` (`guest` / `guest`)
- Prometheus: `http://localhost:9090`
- Grafana: `http://localhost:3000` (`admin` / `admin`)
- MinIO console: `http://localhost:9001` (`minioadmin` / `minioadmin`)
- Alertmanager: `http://localhost:9093`

Submit an immediate job:

```sh
curl -i http://localhost:18080/v1/jobs \
  -H 'Authorization: Bearer fq_admin_dev' \
  -H 'Content-Type: application/json' \
  -d '{"type":"echo","payload":{"message":"hello"},"priority":5,"idempotency_key":"demo-1"}'
```

Submit a timeout/retry demonstration:

```sh
curl -i http://localhost:18080/v1/jobs \
  -H 'Authorization: Bearer fq_admin_dev' \
  -H 'Content-Type: application/json' \
  -d '{"type":"sleep","payload":{"duration":"10s"},"timeout_seconds":2,"max_attempts":3}'
```

Read or cancel a job:

```sh
curl http://localhost:18080/v1/jobs/JOB_ID
curl -X POST http://localhost:18080/v1/jobs/JOB_ID/cancel
```

Scale workers horizontally:

```sh
docker compose up --build --scale worker=3
```

Run the complete portfolio scenario (stack, readiness check, job, and DAG):

```powershell
./scripts/demo.ps1
```

Submit the example DAG:

```sh
go run ./cmd/queuectl workflow examples/workflow.json
```

List dead-lettered jobs and requeue one after correcting its handler/configuration:

```sh
curl 'http://localhost:18080/v1/jobs?status=dead&queue=jobs.default&limit=50'
curl -X POST http://localhost:18080/v1/jobs/JOB_ID/requeue
curl http://localhost:18080/v1/jobs/JOB_ID/attempts
```

List endpoints return `{ "items": [...], "next_cursor": "..." }`. Pass `next_cursor` back as the `cursor` query parameter. Jobs support `status`, `queue`, `type`, `created_after`, and `created_before`; workflows support `status`, `name`, and the same date range. Limits are capped at 100.

Cancel a running job cooperatively and retain the reason:

```sh
curl -X POST http://localhost:18080/v1/jobs/JOB_ID/cancel \
  -H 'Content-Type: application/json' \
  -d '{"reason":"deployment superseded this run"}'
```

Read durable job events or follow them live:

```sh
curl 'http://localhost:18080/v1/jobs/JOB_ID/events?after=0&limit=100'
curl -N http://localhost:18080/v1/jobs/JOB_ID/events/stream
```

Event pages return `items`, `next_after`, and `has_more`. Each job retains its newest 1,000 events; workers publish lifecycle transitions, progress for long-running jobs, and bounded command output. The SSE stream can be resumed with the standard `Last-Event-ID` header or an `after` query parameter.

## API behavior

All `/v1` endpoints require authentication. CLI and service clients use `Authorization: Bearer API_KEY`. The web console sends an API key once to `POST /v1/session`, then uses a short-lived server-side session in a `Secure`, `HttpOnly`, `SameSite=Strict` cookie. Session and CSRF tokens are stored only as SHA-256 digests; unsafe cookie-authenticated requests also require the in-memory `X-CSRF-Token` returned by the session endpoint. Set `WEB_SESSION_SECURE_COOKIE=false` only for local HTTP development; it defaults to `true`.

Docker Compose explicitly runs the migrator with `-seed-development`, which installs the local-only keys `fq_admin_dev`, `fq_operator_dev`, and `fq_viewer_dev`. Normal migrations never create credentials; do not use the seed flag outside disposable local environments. Admins can create principals and one-time API-key tokens through `/v1/admin/principals` and `/v1/admin/principals/{id}/keys`; stored credentials contain only SHA-256 digests.

Retry requests accept `retry_policy` (`fixed`, `linear`, or `exponential`), `retry_initial_seconds`, `retry_max_seconds`, `retry_jitter`, and `non_retryable_errors`.

Workflow steps accept `condition` (`success()`, `failure()`, or `always()`) and `continue_on_error`. A downstream payload can consume an exact upstream value with `${{ steps.STEP_KEY.output.FIELD }}`.

Set `fan_out_from` on a step whose upstream output is a JSON array. ForgeQueue creates one durable child per item, replaces exact `${{ item }}` payload values, aggregates child outputs, and releases fan-in dependents. Add a `compensation` job specification to successful saga steps; a fail-fast workflow schedules those compensations in reverse step order after failure.

Workers accept comma-separated `QUEUE_NAMES` and `WORKER_CAPABILITIES`. Administrators can pause queues or set `max_concurrency` with `PUT /v1/queues/{queue}`. Tenant quotas are managed with `PUT /v1/admin/quotas/{principal_id}`.

Workers drain on SIGTERM: they stop consuming, keep renewing active leases for `WORKER_DRAIN_TIMEOUT`, then release unfinished jobs for fenced retry. The unsafe `command` handler is excluded by default and requires both the `command` capability and `ALLOW_COMMAND_HANDLER=true`.

Artifacts use private S3-compatible storage (MinIO locally). Upload bytes with `POST /v1/jobs/{id}/artifacts?name=output.txt`, list metadata at `GET /v1/jobs/{id}/artifacts`, and request a 15-minute signed download URL from `GET /v1/jobs/{id}/artifacts/{artifactID}/download`. Uploads default to a 50 MiB limit and seven-day retention.

Prometheus loads alerts for queue growth, failure rate, dead letters, workers, leases, scheduler leadership, and the outbox. Kubernetes uses the KEDA RabbitMQ scaler in `deploy/kubernetes/hpa.yaml`. See [security guidance](docs/security.md), [load/chaos testing](docs/load-testing.md), and the [architecture decisions](docs/adr/).

### Slack alert delivery

The default local stack sends alerts only to Alertmanager's no-op development receiver. To enable Slack, create a secret file locally without committing it:

```powershell
New-Item -ItemType Directory -Force .secrets
Set-Content -NoNewline .secrets/slack_webhook_url 'YOUR_NEW_SLACK_WEBHOOK_URL'
docker compose -f compose.yaml -f compose.slack.yaml up -d
```

The webhook is mounted as a Docker secret and read through `api_url_file`; it never appears in Compose configuration or application logs. For CI or deployment, use the existing `SLACK_WEBHOOK_URL` GitHub secret to create the equivalent runtime secret. A GitHub Actions secret alone does not deliver alerts because Alertmanager must be running somewhere.

After pushing the repository, verify the GitHub secret without exposing it by opening **Actions → Test Slack alert → Run workflow**. The manual workflow sends one test message and never prints the webhook.

### Releases

Pushing a semantic-version tag such as `v1.0.0` runs `.github/workflows/release.yml`. It publishes multi-architecture API, worker, scheduler, migrator, CLI, and load-test images under `ghcr.io/usman78654/` and creates a GitHub Release with generated notes.

Scheduling runs in a dedicated service. Multiple replicas are safe: one holds the PostgreSQL session advisory lock while standbys poll for failover. `forgequeue_scheduler_leader` and `forgequeue_scheduler_leadership_transitions_total` expose leadership state.

OpenTelemetry uses W3C Trace Context and exports OTLP/HTTP when `OTEL_EXPORTER_OTLP_ENDPOINT` is configured. Compose includes a collector with a debug exporter for local verification.

`POST /v1/jobs` returns `201` for a new job and `200` for a job previously submitted with the same idempotency key. Scheduled timestamps are RFC 3339 values in `scheduled_at`.

Supported demo job types:

- `echo`: succeeds with its payload as the result.
- `sleep`: accepts `{"duration":"2s"}` and demonstrates timeouts.
- `command`: accepts `{"command":"...","args":[]}`. Use only with trusted inputs.

### Go handler SDK

Application workers can register strict, typed handlers through the public `handler` package. Invalid payloads and errors wrapped with `handler.Permanent` bypass retries and move directly to dead-letter state; cancellation is delivered through the standard Go context.

```go
type ResizeImage struct {
    Source string `json:"source"`
}
type ResizeResult struct {
    Width int `json:"width"`
}

registry := handler.NewRegistry()
handler.RegisterJSON(registry, "resize-image", func(ctx context.Context, input ResizeImage, report handler.Reporter) (ResizeResult, error) {
    report.Progress(0.5, "image decoded")
    report.Log("info", "resize complete", nil)
    return ResizeResult{Width: 1280}, nil
})

err := worker.Run(ctx, worker.Config{
    DatabaseURL: os.Getenv("DATABASE_URL"),
    RabbitMQURL: os.Getenv("RABBITMQ_URL"),
    Queues:      []string{"jobs.images"},
    Handlers:    registry,
})
```

The public worker runner derives capabilities from registered job types, supplies sensible concurrency and lease defaults, and gracefully drains when its context is cancelled. Registries reject duplicate job types and contain handler panics so one bad job cannot terminate a worker loop.

A complete compilable worker is available in `examples/custom-worker`. Handler-managed artifact uploads remain on the production-hardening roadmap.

## Development

The host does not need Go installed; use `make test` or its equivalent:

```sh
docker compose run --rm api go test ./...
```

Versioned, checksummed migrations run through the dedicated `migrate` service before API and worker startup. Migration SQL is stored under `internal/migrations/sql`. For a local database outside Compose, run `go run ./cmd/migrate -seed-development`; omit the flag everywhere else.

Build the React console after frontend changes with `cd frontend && npm ci && npm run build`. Vite writes directly into `internal/httpapi/web`; no copy step is required. Run `npm run quality`, `npm run bundle:check`, and `npm run test:e2e` to verify frontend changes. The committed production bundle keeps normal Go/Docker builds independent of Node.

See [console documentation](docs/console.md) for navigation, operational workflows and troubleshooting, and [the implementation checklist](implementation-plan.md) for remaining development.
