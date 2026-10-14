# Local report workflow demo

Implementation status: COMPLETE. This demo uses bundled JSON fixtures, not external services or accounts.

## Plan

- [x] Register dedicated report handlers on `jobs.reports`, isolated from built-in workers.
- [x] Load customer and order datasets independently, validate and join them, and calculate exact integer-cent sales totals.
- [x] Generate a real Markdown report with deterministic content and retry-safe output files.
- [x] Add success and invalid-input workflow examples and a submission script.
- [x] Test totals, malformed data, unknown customers, cancelled execution and report writing.
- [x] Verify an actual workflow through PostgreSQL, RabbitMQ, API and the custom worker.

```text
Read customers ─┐
                ├→ Validate and aggregate sales → Write Markdown report
Read orders ────┘
```

Output files are local demo files, not S3 artifacts. Object-storage upload remains unavailable when MinIO is not running.

## Run with the current local stack

The API, PostgreSQL, RabbitMQ and scheduler must already be running. From the repository root in PowerShell:

```powershell
$env:FORGEQUEUE_API_KEY = 'fq_admin_dev' # Local seeded credentials only.
./scripts/report-demo.ps1 -StartWorker
./scripts/report-demo.ps1 -InvalidData
```

`-StartWorker` starts only the dedicated worker using `compose.reports.yaml`; it does not pull MinIO or restart the existing stack. This development worker uses the official Go image and shared Go caches; initial compilation can take longer on a fresh machine. The script waits up to two minutes after submission and validates the expected outcome. If it times out, check worker logs and inspect the existing workflow before submitting another run.

The success workflow joins three customers with five orders and creates a real report showing **$485.00** total sales. The failure example intentionally references a missing customer; the aggregate step becomes dead after one attempt despite a three-attempt allowance because the validation error is permanent. The downstream report step is cancelled and no new report is written.

Open `http://localhost:18080/app/#/workflows`, find the `demo-sales-report` workflow and inspect its steps. In Jobs, filter Queue to `jobs.reports` and inspect the `report.render` job to see its result, Markdown, progress and logs. The current DAG builder supports only built-in job types, so submit these custom-handler examples using the script/API, not builder import.

Generated files appear in `demo-output/reports/` on the host. They use a content-derived filename, so identical retries reuse the same report rather than generating duplicates. This is a local demo output directory, not an authenticated artifact download endpoint or a distributed filesystem. Run on one host for this example. Fixture currency is illustrative USD; no live customer information or email delivery is involved.

```powershell
docker compose -f compose.reports.yaml logs report-worker
docker compose -f compose.reports.yaml stop report-worker
```

For a native Go worker, set `DATABASE_URL`, `RABBITMQ_URL` and optionally `REPORT_OUTPUT_DIR`, then run `go run ./examples/report-worker`. Its default output directory is `demo-output/reports`.

## Verification

On 2026-10-14, both examples completed through the live local API, PostgreSQL, RabbitMQ, scheduler and custom worker. The success report contained five orders totaling 48,500 cents; the invalid example failed once and cancelled the report step. Unit/race tests cover deterministic totals, duplicate IDs, missing customers, negative amounts, overflow, cancellation, strict payload decoding and retry-safe file writing. PostgreSQL integration tests run both workflow JSON examples with real output-reference resolution and dependency release.
