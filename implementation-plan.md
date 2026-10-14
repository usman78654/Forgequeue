# ForgeQueue implementation checklist

This is the execution record for the improvements authorized on 2026-10-11. Statuses change only after implementation and verification. The existing frontend and improvement roadmaps remain the detailed acceptance criteria.

Status: PLANNED, IN PROGRESS, COMPLETE, NEEDS INPUT, NEEDS ENVIRONMENT.

| ID | Work | Status | Verification / dependency |
|---|---|---|---|
| I-01 | Query cancellation, request deduplication, cache invalidation, bounded read retries, stale response protection | COMPLETE | API client and concurrency tests; mutation requests must never retry automatically |
| I-02 | Cursor pagination and filters; remove workflow N+1 list loading | IN PROGRESS | Multiple pages without overlap; one list request; details fetched on demand |
| I-03 | Live resumable SSE job progress/logs, attempts and retry timeline | IN PROGRESS | Reconnect and cleanup tests; backend event contract |
| I-04 | Artifact console with upload/download progress, expiry, size and failures | IN PROGRESS | Job detail browser workflows |
| I-05 | Streaming artifact uploads, official S3 client, deletion and lifecycle cleanup | IN PROGRESS | MinIO upload/download/delete and partial failure integration tests |
| I-06 | Worker capabilities, queues, state, heartbeat age, active jobs and filters | IN PROGRESS | API parity and page tests |
| I-07 | Queue depths and routing impact; dead-letter attempt/error comparison | IN PROGRESS | Database counts and operational workflows |
| I-08 | Principal/key creation, quotas and searchable audit administration | IN PROGRESS | Admin RBAC, one-time secret display, confirmation tests |
| I-09 | Stable URLs, resource deep links and back/forward navigation | IN PROGRESS | Browser routing tests |
| I-10 | Advanced DAG editing, validation, deletion, import/export and unsaved changes | IN PROGRESS | Keyboard and workflow validation tests |
| I-11 | Theme tokens, light/dark themes and visual regression coverage | IN PROGRESS | Accessibility and desktop/mobile snapshots |
| I-12 | Configurable observability and runbook links | COMPLETE | Validated external URL configuration |
| I-13 | Drain and artifact integration tests; complete CI gates | IN PROGRESS | Real PostgreSQL, RabbitMQ and MinIO services |
| I-14 | Fresh Compose, automatic chaos and measured load/recovery report | NEEDS ENVIRONMENT | MinIO image pulls denied by registry; record hardware, commit and measured results |
| I-15 | Helm/Kustomize packaging and Kubernetes monitoring/placement policies | IN PROGRESS | Schema and rendered manifest validation |
| I-16 | External secret and managed data-service examples; backup/restore runbook | IN PROGRESS | Provider-neutral templates and restore drills where available |
| I-17 | Cron/time-zone schedules, misfire and overlap policies | PLANNED | DST, catch-up and scheduling integration tests |
| I-18 | Signed durable lifecycle webhooks with retries and delivery history | PLANNED | Signature, retry, deduplication and delivery tests |
| I-19 | Versioned Go API client and maintenance CLI | IN PROGRESS | Typed errors, pagination and operational command tests |
| I-20 | README/roadmap corrections, console docs, architecture diagrams, OpenAPI quality | IN PROGRESS | Examples match current authentication/build; contract validation |
| I-21 | Production TLS/domain and hosted deployment | NEEDS INPUT | Domain, cloud account, budget and certificate/provider choices |
| I-22 | License identity, first release and live Slack delivery | NEEDS INPUT | Copyright identity and release decision; account/deployment access |
| I-23 | Multipart artifacts, worker-generated output artifacts and scanning hooks | PLANNED | Resumable transfer/checksum/abort tests; scanner provider choice for live verification |
| I-24 | Backup-safe object, outbox, lease and workflow reconciliation | PLANNED | Dry-run diagnostics, conservative repairs and recovery integration tests |
| I-25 | Current screenshots, demo recording and evidence-based portfolio summary | PLANNED | Verified runtime evidence; no invented production-scale metrics |

## Execution log

- 2026-10-14: Completed the CI security-remediation batch documented in docs/ci-remediation.md. The supplied 62 reachable findings were addressed by patched Go 1.27.2 and dependency upgrades; exact pinned govulncheck and Trivy scans now pass. Verified all six container builds, Go race/vet/format/mod checks (15.4% coverage), fresh Linux frontend quality/audit/identical embedded build, 20 browser tests and configuration gates. Infrastructure limitations and hosted-run status remain explicit; no scanner suppression or threshold reduction was used.

- 2026-10-14: Fixed the Go-formatting CI failure: six tracked files required gofmt (scheduler/worker entry points, distributed integration tests, broker outbox, scheduler leadership and workflow store). Applied formatting without changing behavior. CI now checks tracked Go files and prints failing filenames instead of suppressing gofmt output; ignored dependencies are excluded without weakening the source formatting gate.

- 2026-10-14: Follow-up CI fix: Trivy v0.28.0 resolves but references the unavailable setup-trivy v0.2.1 internally. Updated to Trivy action v0.36.0 pinned at ed142fd0673e97e23eac54620cfb913e5ce36c25 and explicitly selected scanner v0.70.0. Verified upstream manifests and Git refs for nested setup-trivy v0.2.6, cache v5.0.1/v5.0.5, checkout v6.0.1, the installer script commit and scanner release. HIGH/CRITICAL failures still stop CI; full GitHub execution is not claimed verified locally.

- 2026-10-14: Fixed GitHub Actions setup failure by changing the Trivy action reference from nonexistent `0.28.0` to published `v0.28.0`. Verified the exact tag against the upstream Git repository; scanner configuration and version are otherwise unchanged.

- 2026-10-14 publication checks: frontend typecheck/lint/format, 40 unit/component tests, production build/bundle budgets, all 20 desktop/mobile browser tests, all application Go package-root race tests and vet, Compose validation and staged-file secret scan passed. OpenAPI validated with the two documented warnings. Live MinIO integration remains skipped because TEST_S3_ENDPOINT is unavailable; no claim of full storage verification is made.

- 2026-10-14: Owner requested documentation updates and GitHub publication of the enhancement and report-demo batch. Generated demo output, caches, browser results and secret files are excluded from version control/build context. CI now validates the standalone report-worker Compose configuration. Release tags and unfinished roadmap statuses are intentionally unchanged.

- 2026-10-14: Implemented real local report demos, documented first in examples/report-worker/README.md. Dedicated custom worker loads two bundled datasets independently, validates/joins them, aggregates integer-cent sales, and publishes retry-safe Markdown output. Success and permanent-validation-failure JSON workflows plus a PowerShell runner are provided. Both ran through the live local stack: five orders totaled $485.00; invalid customer failed once and blocked reporting. Dedicated unit/race and PostgreSQL workflow-example tests were added. This does not complete broader I-25 screenshot/video/portfolio criteria.

- 2026-10-11: Recorded the full review before implementation. Begin with shared request architecture, which supports subsequent console features.

### Implemented and verified in this batch

- I-01: GET deduplication, bounded service-error retries, opt-in short cache, mutation invalidation, abort propagation and stale-response protection. Writes never retry automatically.
- I-02: Jobs/workflows cursor navigation, debounced job text filters, one workflow list request and on-demand step details. More multi-page browser coverage remains.
- I-03: Resumable SSE logs/progress, deduplication, disconnect cleanup and attempt history. Live terminal status synchronization and expanded reconnect tests remain.
- I-04/I-05: Artifact metadata, upload pending/errors, signed downloads, confirmed deletion, object-first cleanup and orphan cleanup on metadata failure. Percentage progress, streaming/official S3 client and live MinIO verification remain.
- I-06/I-07: Worker capabilities/queues/state/heartbeat age/filter, active-work counts and ready queue depths. Routing explanation, worker refresh coverage and DLQ attempt comparison remain.
- I-08: Principal/key creation, one-time secret, key-table refresh, confirmed quotas and newest-200 audit search. Principal listing, audit pagination, expanded admin tests and expiry controls remain.
- I-09/I-10: Page hash URLs, browser navigation, DAG edit/delete/import/export/cycle checks, close-tab/in-app unsaved warning and viewer submission guard. Builder tests cover cyclic imports and navigation cancellation. Resource deep links, layout editing and expanded import/export coverage remain.
- I-11/I-12: Light/dark themes and validated configurable metrics/traces/runbook links. Theme accessibility/visual baselines and resource-specific observability links remain.
- I-15/I-16: Staging/production Kustomize, security/placement policies, bounded writable scratch space, scheduler metrics service/PDB, ServiceMonitors and PrometheusRule alerts; render checks pass. External-secret examples, schema validation and actual restore drills remain.
- I-19/I-20: Initial versioned Go client with typed jobs, cursor pages, context cancellation, typed errors, bounded reads, redirect protection and write-no-retry semantics (four tests pass with race detection). Maintenance CLI authentication/timeouts/bounded reads and added operations; current console, client, contribution, changelog and disaster-recovery docs. OpenAPI validates with two warnings (undecided license and localhost development URL). Typed non-job client operations, CLI tests and architecture diagrams remain.
- PostgreSQL/RabbitMQ-backed Go tests passed after development migrations. Artifact integration test was added but skipped without TEST_S3_ENDPOINT. MinIO registry access returned authorization errors; live S3 verification and full fresh-stack/chaos/load evidence remain blocked.
- Frontend quality passed 40 tests across 14 files; production chunks pass the existing size budgets. All 20 desktop/mobile browser tests pass. All application Go package roots (commands, internals, client, integration, handler, worker and examples) pass race-enabled tests and vet against PostgreSQL/RabbitMQ. Staging/production each validate 16 built-in resources, skip two custom resources, and report zero errors; three monitoring custom resources render but lack local schemas. CI now validates rendered overlays.
- I-17/I-18 have not started; cron/time-zone semantics and durable signed webhooks require schema, implementation and dedicated integration coverage. These are not claimed complete.
- I-13: Added PostgreSQL drain tests for attempt closure, idempotent release, stale-result fencing, restarted worker state and preservation of completed work. Fixed a real connection-busy ordering bug in ReleaseWorkerJobs and removed ignored attempt-update errors. Controlled storage-failure cleanup tests also pass with race detection. Real process shutdown/readiness and live MinIO protocol verification remain outstanding.
- Added admin tests for one-time secret removal, no browser-storage persistence, key metadata refresh and quota confirmation. CLI now rejects redirects and oversized responses instead of silently truncating them.
- Replaced obsolete architecture milestones/state diagrams with current system, job and workflow diagrams and explicit runtime limitations. Publication is tracked in Git history and origin/main; unfinished roadmap items remain explicit.

## Next implementation sequence

1. Finish frontend acceptance gaps: resource URLs, multi-page browser coverage, live terminal/attempt refresh, transfer percentages and theme/visual regression checks.
2. Finish storage hardening and real process-drain verification; resolve MinIO registry availability before claiming end-to-end storage or full-stack verification.
3. Add cron/time-zone policies and durable webhooks with migrations and dedicated failure/DST/retry integration coverage.
4. Complete typed SDK coverage, maintenance reconciliation, secret-provider templates, restore drills and measured load/chaos evidence.
5. Keep hosting, domain/TLS, license, release and live account integrations gated on owner choices.

## Handoff requirements

Record tests actually run, failures and remaining dependencies. Do not claim infrastructure integration tests passed when they were skipped. Each unfinished item above remains visible for future continuation.
