# Load baseline

Status: pending execution on the target machine.

Command: `go run ./cmd/loadtest -jobs 5000 -concurrency 100 -timeout 5m`

Record the date, commit, CPU/RAM, container runtime, worker replicas/concurrency, output JSON, peak queue depth, database CPU, RabbitMQ publish/ack rate, and any alerts. This repository does not publish fabricated benchmark numbers; CI verifies the runner, while performance results are hardware-specific.
