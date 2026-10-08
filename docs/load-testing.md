# Load and chaos testing

Start the stack, export `FORGEQUEUE_API_KEY` when authentication is enabled, and run:

```powershell
go run ./cmd/loadtest -jobs 5000 -concurrency 100 -timeout 5m
./scripts/chaos.ps1 worker
./scripts/chaos.ps1 broker
./scripts/chaos.ps1 database
```

The load runner reports accepted/completed counts, end-to-end duration, throughput, and p50/p95/p99 submission latency as machine-readable JSON. Keep the API, worker count, CPU/memory, PostgreSQL/RabbitMQ versions, payload, and host hardware fixed when comparing runs. Save results under `docs/reports/`; the checked-in baseline is a reproducible template because performance numbers from a different machine would be misleading.

Each chaos scenario submits durable work, interrupts one dependency, restores it, and requires the job to reach `succeeded`. The worker case exercises lease recovery, the broker case exercises the transactional outbox, and the database case exercises client reconnection and authoritative-state recovery. Run against the local Compose project only—the script intentionally stops services.
