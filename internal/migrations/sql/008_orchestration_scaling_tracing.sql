ALTER TYPE workflow_status ADD VALUE IF NOT EXISTS 'compensating';
ALTER TYPE workflow_status ADD VALUE IF NOT EXISTS 'compensated';

ALTER TABLE workflow_steps ADD COLUMN fan_out_from text;
ALTER TABLE workflow_steps ADD COLUMN parent_step_key text;
ALTER TABLE workflow_steps ADD COLUMN compensation jsonb;
ALTER TABLE workflow_steps ADD COLUMN is_compensation boolean NOT NULL DEFAULT false;
CREATE INDEX workflow_steps_parent_idx ON workflow_steps(workflow_id,parent_step_key) WHERE parent_step_key IS NOT NULL;

ALTER TABLE jobs ADD COLUMN required_capabilities text[] NOT NULL DEFAULT '{}';
ALTER TABLE jobs ADD COLUMN traceparent text;
ALTER TABLE outbox ADD COLUMN traceparent text;
ALTER TABLE workflows ADD COLUMN traceparent text;
CREATE FUNCTION forgequeue_inherit_trace() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.traceparent IS NULL AND NEW.workflow_id IS NOT NULL THEN SELECT traceparent INTO NEW.traceparent FROM workflows WHERE id=NEW.workflow_id; END IF; RETURN NEW; END $$;
CREATE TRIGGER jobs_inherit_trace BEFORE INSERT ON jobs FOR EACH ROW EXECUTE FUNCTION forgequeue_inherit_trace();
CREATE FUNCTION forgequeue_outbox_trace() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.traceparent IS NULL THEN SELECT traceparent INTO NEW.traceparent FROM jobs WHERE id=NEW.job_id; END IF; RETURN NEW; END $$;
CREATE TRIGGER outbox_inherit_trace BEFORE INSERT ON outbox FOR EACH ROW EXECUTE FUNCTION forgequeue_outbox_trace();

ALTER TABLE workers ADD COLUMN queues text[] NOT NULL DEFAULT '{}';
ALTER TABLE workers ADD COLUMN capabilities text[] NOT NULL DEFAULT '{}';

CREATE TABLE queue_controls (
    queue text PRIMARY KEY,
    paused boolean NOT NULL DEFAULT false,
    max_concurrency integer CHECK(max_concurrency IS NULL OR max_concurrency > 0),
    updated_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE recurring_schedules ADD COLUMN owner_id uuid REFERENCES principals(id) ON DELETE CASCADE;

CREATE TABLE tenant_quotas (
    principal_id uuid PRIMARY KEY REFERENCES principals(id) ON DELETE CASCADE,
    submissions_per_minute integer NOT NULL DEFAULT 120 CHECK(submissions_per_minute > 0),
    max_concurrent_jobs integer NOT NULL DEFAULT 50 CHECK(max_concurrent_jobs > 0),
    max_payload_bytes integer NOT NULL DEFAULT 1048576 CHECK(max_payload_bytes > 0),
    max_schedules integer NOT NULL DEFAULT 25 CHECK(max_schedules > 0),
    updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO tenant_quotas(principal_id) SELECT id FROM principals ON CONFLICT DO NOTHING;

CREATE TABLE scheduler_leases (
    name text PRIMARY KEY,
    holder_id text NOT NULL,
    acquired_at timestamptz NOT NULL DEFAULT now(),
    heartbeat_at timestamptz NOT NULL DEFAULT now()
);
