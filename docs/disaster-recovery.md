# Backup and recovery

PostgreSQL is the source of truth for jobs, leases, attempts, workflows, outbox messages, ownership and artifact metadata. RabbitMQ deliveries can be duplicated; restoring the broker alone does not restore application state.

## Production policy

Choose and record an RPO and RTO before deployment. A starting target for a small deployment is RPO 15 minutes and RTO one hour, subject to measured restore drills. Use managed PostgreSQL point-in-time recovery or continuous WAL archiving plus daily base backups. Encrypt backups and store them in a separate account or failure domain. Retain a minimum of seven daily and four weekly copies according to business requirements.

Enable object-storage versioning and lifecycle retention consistent with database backups. Restoring older database metadata can refer to deleted artifacts, so object versions must cover the entire database recovery window. Keep credentials and encryption keys in the chosen secret manager, backed up separately with restricted recovery access.

## Restore drill

1. Provision an isolated database, private object bucket and broker. Prevent consumers from connecting until checks finish.
2. Restore PostgreSQL to the chosen timestamp and restore matching object versions. Record backup timestamp, duration and identifiers without secret values.
3. Run the normal checksummed migration command without development seeding. A checksum mismatch is a stop condition.
4. Start the scheduler, API and one worker. Expired leases are reclaimed through the existing fencing logic; never manually declare running jobs successful.
5. Inspect outbox backlog, queued/dead/running job counts, workflow steps and missing artifact objects. Re-submit externally significant jobs only after confirming their idempotency semantics.
6. Submit an echo job, a multi-step workflow and an artifact. Verify results, signed download and cancellation. Compare restore duration to the RTO.
7. Document observed data loss relative to the RPO and resolve discrepancies before production recovery is accepted.

## Migration rollback

SQL migrations are forward-only and checksummed. Roll back application images only when they remain compatible with the current schema. For incompatible schema failures, apply a reviewed forward repair or restore an isolated backup and perform a cutover; do not delete migration records or edit applied migration files.

## Data services

Use a managed PostgreSQL service with TLS and backups, managed RabbitMQ or a supported operator with persistent volumes, and private S3-compatible storage. Configure connection limits so API, workers and scheduler replicas fit the database capacity. Provide DATABASE_URL, RABBITMQ_URL, S3_ACCESS_KEY and S3_SECRET_KEY through your secret manager; do not commit populated Kubernetes secrets.

A successful live restore drill and measured RPO/RTO remain environment-dependent acceptance criteria.
