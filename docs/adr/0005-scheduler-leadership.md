# ADR 0005: PostgreSQL scheduler leadership

Status: accepted

Scheduler replicas use a PostgreSQL advisory lock, so only one releases scheduled jobs while standbys remain ready for failover. This avoids another coordination dependency and matches the authoritative store. Very large scheduling workloads may eventually justify partitioned leadership.
