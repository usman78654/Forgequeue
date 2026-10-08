# ADR 0003: Transactional outbox

Status: accepted

Submission writes the job and an outbox record in one database transaction. A publisher retries RabbitMQ delivery and marks the record published. This removes the database/broker dual-write gap. It adds delivery latency and duplicate publication, both handled by short polling and fenced claims.
