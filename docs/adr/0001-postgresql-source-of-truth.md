# ADR 0001: PostgreSQL is the source of truth

Status: accepted

Job, attempt, workflow, schedule, ownership, artifact metadata, and lease state live in PostgreSQL. RabbitMQ is a delivery accelerator, not the authoritative ledger. This makes recovery inspectable and permits transactional state changes, at the cost of database write load and a scheduler/outbox process.
