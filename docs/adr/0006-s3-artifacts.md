# ADR 0006: S3-compatible artifact storage

Status: accepted

Large logs and outputs belong in private S3-compatible object storage; PostgreSQL keeps searchable metadata and retention state. Local development uses MinIO. Downloads use short-lived signed URLs, uploads have a hard size limit, and cleanup removes expired metadata and objects. This keeps the database lean while remaining portable across cloud providers.
