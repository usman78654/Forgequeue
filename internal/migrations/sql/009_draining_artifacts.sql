ALTER TABLE workers ADD COLUMN state text NOT NULL DEFAULT 'running' CHECK(state IN ('running','draining','offline'));
ALTER TABLE workers ADD COLUMN draining_at timestamptz;

CREATE TABLE job_artifacts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id uuid NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    name text NOT NULL CHECK(length(name) BETWEEN 1 AND 255),
    content_type text NOT NULL DEFAULT 'application/octet-stream',
    object_key text NOT NULL UNIQUE,
    size_bytes bigint NOT NULL CHECK(size_bytes >= 0 AND size_bytes <= 52428800),
    sha256 text NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX job_artifacts_job_idx ON job_artifacts(job_id,created_at DESC);
CREATE INDEX job_artifacts_expiry_idx ON job_artifacts(expires_at);
