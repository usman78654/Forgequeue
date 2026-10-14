# CI security remediation — 2026-10-14

Status: IMPLEMENTED AND LOCALLY VERIFIED. Hosted run outcomes are recorded in GitHub Actions; local results below do not imply that a hosted run has already completed.

The supplied govulncheck report lists 62 reachable vulnerabilities in the Go 1.23 standard library and dependencies. Highest reported fix floors include Go 1.26.9, x/text v0.41.0, x/net v0.60.0, OpenTelemetry v1.45.0, amqp091-go v1.13.0, gRPC v1.83.1 and pgx v5.9.2.

## Checklist

- [x] Verify released versions and align the Go module, CI, Docker builder and report worker on a supported patched toolchain.
- [x] Update vulnerable direct/transitive modules and regenerate go.sum with normal Go tooling.
- [x] Pin the vulnerability-check tool and rerun source and dependency scans without suppressions.
- [x] Check frontend audit and the remaining CI gates, including Linux embedded-bundle reproducibility, coverage, configuration and container builds.
- [x] Run formatting, vet, race-enabled PostgreSQL/RabbitMQ integration tests and frontend tests after updates.
- [x] Record actual results and remaining infrastructure limits; prepare the verified batch for publication.

Security thresholds remain unchanged. No vulnerability ignore list, continue-on-error or disabled scanner is introduced. Live MinIO tests still require a working object-storage service and are not claimed verified unless run.

## Versions and results

Go is pinned to **1.27.2**, verified against the [official stable releases](https://go.dev/dl/). CI reads the version from go.mod; Docker and the report-worker configuration use the same patch. govulncheck is pinned to **v1.9.0** rather than `latest`.

| Dependency | Previous | Updated |
|---|---|---|
| pgx | 5.7.2 | 5.11.0 |
| RabbitMQ Go client | 1.10.0 | 1.15.0 |
| OpenTelemetry | 1.34.0 | 1.47.0 |
| gRPC | 1.69.4 | 1.84.0 |
| x/net | 0.34.0 | 0.61.0 |
| x/text | 0.21.0 | 0.43.0 |
| Prometheus Go client | 1.20.5 | 1.25.0 |
| klauspost/compress | 1.17.9 | 1.20.3 |

Normal `go get -u`/`go mod tidy` updated the full selected dependency graph, not just the first reported module. Obsolete dependencies no longer required by the updated libraries were removed.

Verified on 2026-10-14:

- `govulncheck ./...` with the exact pinned tool on a clean source snapshot: **No vulnerabilities found**.
- Trivy 0.70.0 filesystem scan, with vulnerability and secret scanning enabled and HIGH/CRITICAL exit-code 1: **0 findings** for go.mod and the frontend lockfile.
- All application Go package roots: formatting, vet, race-enabled tests and PostgreSQL/RabbitMQ integration pass. Coverage: **15.4%**, above the unchanged **5%** gate.
- `go mod verify`, `go mod tidy -diff` and all command/example builds pass.
- Fresh Linux `npm ci`: typecheck, lint, format, **40 tests**, build, bundle budgets and audit pass; **0 npm vulnerabilities**. Linux bundle matches the committed embedded files byte-for-byte.
- **20** desktop/mobile browser tests pass.
- API, worker, scheduler, migrator, queuectl and loadtest Docker images all build with the patched toolchain.
- Base/report/Slack Compose validation, Prometheus configuration and seven alert rules, both Alertmanager configurations, and Kubernetes renders/schema checks pass. Missing custom-resource schemas are still explicitly skipped as in the existing CI configuration.
- OpenAPI validates with two existing non-blocking warnings: undecided license and localhost development URL.

Existing long-running local preview containers are not automatically replaced by a Git push. Rebuild/recreate them with the updated configuration when restarting the preview. No production deployment or release tag is created by this remediation.
