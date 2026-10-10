# ForgeQueue Improvement Roadmap

This roadmap begins after the core feature roadmap in `FEATURES.md`. It tracks the remaining work needed to turn ForgeQueue from a feature-complete portfolio implementation into a polished, demonstrably production-ready project.

Status legend:

- `PLANNED` — not started
- `IN PROGRESS` — currently being implemented
- `COMPLETE` — implemented and verified
- `NEEDS ENVIRONMENT` — implementation exists or can proceed, but final verification requires Docker, Kubernetes, or suitable infrastructure
- `NEEDS USER INPUT` — requires account access, credentials, a domain, branding, or another user-owned decision

Autonomy legend:

- `YES` — Codex can implement and locally verify this independently
- `MOSTLY` — Codex can implement it, but final verification or configuration needs an external environment
- `PARTIAL` — meaningful user-owned information or access is required

## Priority 1 — Release blockers

| # | Improvement | Status | Autonomy | Completion criteria |
|---|---|---|---|---|
| 1 | Full Docker Compose verification | NEEDS ENVIRONMENT | MOSTLY | Fresh migration, API, workers, scheduler, RabbitMQ, MinIO, Prometheus, Grafana, and tracing start successfully; job, workflow, artifact, retry, cancellation, and recovery smoke tests pass |
| 2 | Graceful-drain integration tests | PLANNED | YES | Tests prove active jobs finish during draining, timed-out drains release work once, readiness becomes unavailable, and restarted workers return to `running` |
| 3 | Artifact integration tests | PLANNED | YES | Real MinIO tests cover upload, listing, signed download, ownership, size rejection, expiry, cleanup, and missing objects |
| 4 | Prometheus and manifest validation | COMPLETE | YES | CI runs `promtool`, Compose validation, OpenAPI validation, Kubernetes schema validation, and KEDA resource checks |
| 5 | Real load-test baseline | NEEDS ENVIRONMENT | MOSTLY | Reproducible test is run against the complete stack and records throughput, p50/p95/p99 latency, queue depth, resources, configuration, hardware, and commit |
| 6 | Automated chaos verification | NEEDS ENVIRONMENT | MOSTLY | Worker, RabbitMQ, and PostgreSQL interruption scenarios run automatically and prove eventual recovery without lost terminal state |

## Priority 2 — Repository and CI/CD

| # | Improvement | Status | Autonomy | Completion criteria |
|---|---|---|---|---|
| 7 | Initialize and clean Git repository | COMPLETE | YES | Repository is initialized on `main`, generated dependencies and secrets are ignored, the GitHub remote is configured, and the initial commit is ready to push |
| 8 | Replace placeholder module and image names | COMPLETE | PARTIAL | Go imports use `github.com/usman78654/Forgequeue` and deployment images use `ghcr.io/usman78654/` |
| 9 | License, contribution guide, and changelog | PLANNED | YES | Repository contains an approved license, `CONTRIBUTING.md`, release process, semantic-versioning guidance, and `CHANGELOG.md` |
| 10 | Remove production development credentials | COMPLETE | YES | Development API keys are created only in an explicit local-development mode and cannot appear in production migrations |
| 11 | Complete CI build matrix | COMPLETE | YES | CI builds API, worker, scheduler, migrator, CLI, load tester, frontend, and all container targets |
| 12 | CI quality gates | IN PROGRESS | YES | CI enforces formatting, vetting, race tests, coverage threshold, vulnerability checks, frontend audit, OpenAPI/YAML/Prometheus validation, and artifact integration tests |
| 13 | Container publishing and releases | NEEDS ENVIRONMENT | PARTIAL | Tag-driven workflow publishes multi-architecture GHCR images with provenance and a generated GitHub Release; first tag still needs live verification |

## Priority 3 — Production deployment

| # | Improvement | Status | Autonomy | Completion criteria |
|---|---|---|---|---|
| 14 | Helm chart or Kustomize overlays | PLANNED | YES | Reusable development, staging, and production configuration supports images, replicas, resources, endpoints, ingress, autoscaling, and observability |
| 15 | TLS ingress | NEEDS USER INPUT | PARTIAL | Ingress configuration uses the chosen domain, certificate issuer, secure redirects, HSTS, and documented DNS setup |
| 16 | External secret management | PLANNED | MOSTLY | External Secrets/Vault/cloud-secret example replaces inline Kubernetes secrets; live verification uses the selected provider |
| 17 | Production data-service examples | PLANNED | MOSTLY | Documentation and values cover managed or operator-backed PostgreSQL, RabbitMQ, and S3/MinIO deployments |
| 18 | Kubernetes observability resources | PLANNED | YES | `ServiceMonitor`, `PodMonitor`, and `PrometheusRule` resources expose all services and alert rules |
| 19 | Alertmanager routing | NEEDS ENVIRONMENT | PARTIAL | Alertmanager and secret-file Slack routing are configured; delivery needs a running deployment and the repository's `SLACK_WEBHOOK_URL` secret |
| 20 | Availability and placement policies | PLANNED | YES | Deployments include topology spread, pod anti-affinity, disruption budgets, priorities, security contexts, and tuned termination grace periods |
| 21 | Backup and disaster recovery | PLANNED | MOSTLY | Backup, point-in-time recovery, artifact lifecycle, restore drills, migration rollback policy, RPO, and RTO are documented and tested where infrastructure permits |

## Priority 4 — Runtime hardening

| # | Improvement | Status | Autonomy | Completion criteria |
|---|---|---|---|---|
| 22 | Streaming artifact uploads | PLANNED | YES | API streams bounded uploads without buffering the complete object in memory and cleans up partial failures |
| 23 | Multipart artifact support | PLANNED | YES | Large artifacts use resumable multipart uploads with checksums and abort cleanup |
| 24 | Official S3 client integration | PLANNED | YES | Custom signing code is replaced by the official AWS or MinIO SDK with endpoint, TLS, retry, and credential-provider support |
| 25 | Worker-generated log and output artifacts | PLANNED | YES | Handlers can stream large logs/results to object storage while PostgreSQL retains searchable metadata and bounded event previews |
| 26 | Artifact deletion and lifecycle | PLANNED | YES | Authorized deletion, retention policies, object lifecycle rules, orphan reconciliation, and audit events are implemented |
| 27 | Upload security scanning hooks | PLANNED | MOSTLY | Configurable quarantine and malware/content scanning workflow exists; live scanning depends on the selected scanner |
| 28 | Backup-safe state reconciliation | PLANNED | YES | Maintenance commands detect and reconcile orphaned objects, stale outbox records, abandoned leases, and inconsistent workflow state |

## Priority 5 — Product capabilities

| # | Improvement | Status | Autonomy | Completion criteria |
|---|---|---|---|---|
| 29 | Signed webhooks | PLANNED | YES | Job/workflow lifecycle webhooks support signing, idempotency, retries, delivery history, endpoint disablement, and dead-letter handling |
| 30 | Cron and time-zone scheduling | PLANNED | YES | Schedules support cron expressions, IANA time zones, misfire policy, catch-up/skip behavior, overlap limits, and execution history |
| 31 | Go handler SDK | IN PROGRESS | YES | Typed handler registry provides payload validation, progress, structured logs, artifacts, cancellation, heartbeats, and permanent/retryable errors |
| 32 | Client SDK | PLANNED | YES | A versioned Go client covers jobs, workflows, schedules, events, artifacts, queues, pagination, retries, and typed API errors |
| 33 | Web console feature parity | PLANNED | YES | Console supports artifacts, live SSE logs, dead-letter operations, queue controls, worker draining state, retry timelines, metrics links, audit history, and secure key setup |
| 34 | Administrative maintenance CLI | PLANNED | YES | CLI supports diagnostics, queue control, worker inspection, DLQ requeue, artifact cleanup, state reconciliation, and configuration checks |

## Priority 6 — Portfolio presentation

| # | Improvement | Status | Autonomy | Completion criteria |
|---|---|---|---|---|
| 35 | Polished architecture diagrams | PLANNED | YES | README contains clear system, job lifecycle, workflow, failure-recovery, and deployment diagrams |
| 36 | Screenshots and demonstration recording | NEEDS ENVIRONMENT | MOSTLY | Repository contains current dashboard screenshots and a short recording showing execution, scaling, retry, cancellation, and crash recovery |
| 37 | Measured results and engineering narrative | NEEDS ENVIRONMENT | MOSTLY | README presents honest benchmark results, recovery timing, tested scale, limitations, and design tradeoffs |
| 38 | Hosted demonstration | NEEDS USER INPUT | PARTIAL | A secured, cost-controlled deployment is available through a user-owned cloud account and domain |
| 39 | Resume-ready project summary | PLANNED | YES | Concise project description, architecture highlights, verified metrics, responsibilities, and interview talking points are documented |

## Recommended implementation order

1. Add integration tests for draining and artifacts.
2. Complete CI validation and build coverage.
3. Replace development credentials with explicit development seeding.
4. Build the Helm chart and Kubernetes monitoring resources.
5. Harden artifact streaming and adopt the official S3 SDK.
6. Add the handler SDK, webhooks, and cron scheduling.
7. Bring the web console to backend feature parity.
8. Run Compose, chaos, load, and recovery verification when Docker is available.
9. Replace repository placeholders and publish signed images after GitHub ownership is supplied.
10. Capture measured results, screenshots, and the final demonstration.

## Deferred user actions

These actions are intentionally postponed and are not blockers for local development:

| Action | Status | Instructions |
|---|---|---|
| Rotate the exposed Slack webhook | DEFERRED — SECURITY REQUIRED | Revoke the webhook previously shared in chat, generate a replacement, and update the `SLACK_WEBHOOK_URL` GitHub Actions secret. Never send or commit the replacement URL. |
| Correct the initial commit author email | DEFERRED | Get the GitHub no-reply address from **GitHub Settings → Emails**, set it with `git config user.email`, then run `git commit --amend --reset-author --no-edit` before pushing. |
| Push the local `main` branch | DEFERRED | Authenticate with GitHub, then run `git push -u origin main`. The configured remote is `https://github.com/usman78654/Forgequeue.git`. |
| Verify the Slack secret | DEFERRED | After pushing, run **Actions → Test Slack alert → Run workflow** and confirm that one message reaches `#forgequeue-alerts`. |
| Publish the first release | DEFERRED | After CI passes, create and push `v1.0.0`; the release workflow will publish GHCR images and create the GitHub Release. |
| Add the MIT license | DEFERRED | Add GitHub's standard MIT `LICENSE` template using the preferred copyright name. |
