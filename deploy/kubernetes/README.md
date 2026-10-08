# Kubernetes deployment

These manifests deploy the stateless API, worker, and leader-elected scheduler layers. PostgreSQL and RabbitMQ should use managed services or production-grade operators; their lifecycle does not belong in the application deployment.

1. Replace both `CHANGE_ME` values and the `OWNER` image paths.
2. Run migrations as a release job before rolling out the API.
3. Apply with `kubectl apply -f deploy/kubernetes/`.

Install KEDA before applying the manifests. The included `ScaledObject` scales workers from the `jobs.default` RabbitMQ queue depth. Apply `network-policy.yaml` only with a policy-capable CNI and tailor its namespace selectors to your infrastructure. Configure the S3 settings for a private MinIO/S3 bucket and route `S3_PUBLIC_ENDPOINT` through TLS.

Run at least two scheduler replicas. A PostgreSQL advisory lock elects exactly one active scheduler while standbys wait for automatic failover. Configure workers with `QUEUE_NAMES` and `WORKER_CAPABILITIES`, and configure all services with the same OTLP collector endpoint when tracing is enabled.
