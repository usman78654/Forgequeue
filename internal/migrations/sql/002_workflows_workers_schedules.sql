CREATE TYPE workflow_status AS ENUM ('running','succeeded','failed','cancelled');
CREATE TYPE workflow_step_status AS ENUM ('pending','queued','running','succeeded','failed','cancelled');

CREATE TABLE workflows (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL,
    status workflow_status NOT NULL DEFAULT 'running',
    fail_fast boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz
);

CREATE TABLE workflow_steps (
    workflow_id uuid NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    step_key text NOT NULL,
    job_type text NOT NULL,
    queue text NOT NULL DEFAULT 'jobs.default',
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    dependencies text[] NOT NULL DEFAULT '{}',
    status workflow_step_status NOT NULL DEFAULT 'pending',
    priority smallint NOT NULL DEFAULT 0,
    max_attempts integer NOT NULL DEFAULT 3,
    timeout_seconds integer NOT NULL DEFAULT 300,
    job_id uuid REFERENCES jobs(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(workflow_id, step_key)
);
CREATE UNIQUE INDEX workflow_steps_job_uq ON workflow_steps(job_id) WHERE job_id IS NOT NULL;
ALTER TABLE jobs ADD COLUMN workflow_id uuid REFERENCES workflows(id) ON DELETE CASCADE;
ALTER TABLE jobs ADD COLUMN workflow_step_key text;

CREATE TABLE workers (
    id text PRIMARY KEY,
    queue text NOT NULL,
    concurrency integer NOT NULL,
    started_at timestamptz NOT NULL DEFAULT now(),
    last_heartbeat_at timestamptz NOT NULL DEFAULT now(),
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX workers_heartbeat_idx ON workers(last_heartbeat_at);

CREATE TABLE recurring_schedules (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL UNIQUE,
    job_type text NOT NULL,
    queue text NOT NULL DEFAULT 'jobs.default',
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    interval_seconds integer NOT NULL CHECK(interval_seconds > 0),
    priority smallint NOT NULL DEFAULT 0,
    max_attempts integer NOT NULL DEFAULT 3,
    timeout_seconds integer NOT NULL DEFAULT 300,
    enabled boolean NOT NULL DEFAULT true,
    next_run_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX recurring_due_idx ON recurring_schedules(next_run_at) WHERE enabled;
