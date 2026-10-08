# Security guide

ForgeQueue defaults to API-key authentication, tenant ownership checks, audit records, bounded request bodies, security headers, and a safe `echo,sleep` worker registry. The arbitrary `command` handler is disabled unless `ALLOW_COMMAND_HANDLER=true`; never enable it in a multi-tenant deployment. Add application-specific handlers as compiled code with validated payload schemas.

## Production checklist

- Terminate TLS 1.2+ at an ingress/load balancer, redirect HTTP, and keep HSTS enabled. Use `postgres://...?sslmode=verify-full`, `amqps://`, and HTTPS S3 endpoints between services.
- Create credentials in a secret manager (External Secrets, Vault, or cloud workload identity). Do not apply the placeholder `stringData` values from `namespace.yaml`; rotate API, database, RabbitMQ, and S3 keys independently.
- Apply `network-policy.yaml` with a CNI that enforces policies, then narrow the example namespace selectors to the actual database, broker, storage, ingress, and monitoring namespaces.
- Run containers as non-root (the supplied distroless image does), use a read-only root filesystem, pin image digests, and sign release images.
- Keep artifact buckets private. Downloads are time-limited signed URLs; configure object-store lifecycle deletion as a second layer behind the database cleanup worker.
- Review audit events and Prometheus alerts. Never put credentials or sensitive payloads in labels, logs, idempotency keys, or job errors.

CI runs Go vulnerability analysis and filesystem/container scans. Dependabot keeps Go, npm, Actions, and Docker dependencies visible for review.

Slack webhook URLs are credentials. Store them in GitHub Actions Secrets or a secret manager and expose them to Alertmanager as a file. Never place a webhook URL in tracked YAML, `.env.example`, documentation, screenshots, commits, or issue descriptions. Revoke and replace any URL that has been pasted into chat or another non-secret channel.
