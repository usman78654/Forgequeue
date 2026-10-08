CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TYPE job_status AS ENUM ('scheduled','queued','running','succeeded','retrying','failed','dead','cancelled');

CREATE TABLE jobs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    type text NOT NULL,
    queue text NOT NULL DEFAULT 'jobs.default',
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    status job_status NOT NULL,
    priority smallint NOT NULL DEFAULT 0 CHECK (priority BETWEEN 0 AND 9),
    idempotency_key text,
    max_attempts integer NOT NULL DEFAULT 3 CHECK (max_attempts > 0),
    attempt integer NOT NULL DEFAULT 0,
    timeout_seconds integer NOT NULL DEFAULT 300 CHECK (timeout_seconds > 0),
    scheduled_at timestamptz NOT NULL DEFAULT now(),
    lease_expires_at timestamptz,
    worker_id text,
    result jsonb,
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz
);
CREATE UNIQUE INDEX jobs_idempotency_key_uq ON jobs (idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX jobs_schedule_idx ON jobs (scheduled_at, priority DESC) WHERE status IN ('scheduled','retrying');
CREATE INDEX jobs_lease_idx ON jobs (lease_expires_at) WHERE status = 'running';

CREATE TABLE job_attempts (
    id bigserial PRIMARY KEY,
    job_id uuid NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    attempt integer NOT NULL,
    worker_id text NOT NULL,
    started_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    error text,
    UNIQUE(job_id, attempt)
);

CREATE TABLE outbox (
    id bigserial PRIMARY KEY,
    job_id uuid NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    queue text NOT NULL,
    priority smallint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz
);
CREATE INDEX outbox_pending_idx ON outbox (id) WHERE published_at IS NULL;
