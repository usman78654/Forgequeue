# Kubernetes deployment

These manifests deploy the stateless API, worker, and leader-elected scheduler layers. PostgreSQL and RabbitMQ should use managed services or production-grade operators; their lifecycle does not belong in the application deployment.

1. Provide the real data-service endpoints and secrets. Replace placeholder `CHANGE_ME` values and pin images to an approved release tag or digest.
2. Run migrations as a release job before rolling out the API.
3. Render with `kubectl kustomize deploy/overlays/production` and apply with `kubectl apply -k deploy/overlays/production` after reviewing the output. Staging uses `deploy/overlays/staging`.

Install KEDA before applying the manifests. The included `ScaledObject` scales workers from the `jobs.default` RabbitMQ queue depth. Apply `network-policy.yaml` only with a policy-capable CNI and tailor its namespace selectors to your infrastructure. Configure the S3 settings for a private MinIO/S3 bucket and route `S3_PUBLIC_ENDPOINT` through TLS.

Run at least two scheduler replicas. A PostgreSQL advisory lock elects exactly one active scheduler while standbys wait for automatic failover. Configure workers with `QUEUE_NAMES` and `WORKER_CAPABILITIES`, and configure all services with the same OTLP collector endpoint when tracing is enabled.

Install Prometheus Operator before applying `kubectl apply -k deploy/monitoring`. Configure its ServiceMonitor and PrometheusRule namespace/label selectors to discover the provided monitors and alerts. Production overlays add non-root execution, a read-only filesystem with a bounded writable `/tmp`, dropped capabilities, topology spreading and a 60-second shutdown allowance.

The base secret contains illustrative placeholders and must be replaced by an externally managed Secret before deployment. See [backup and recovery](../../docs/disaster-recovery.md). TLS ingress, cluster-specific network policy and secret-provider configuration require deployment choices.
