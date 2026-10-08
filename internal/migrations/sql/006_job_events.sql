CREATE TABLE job_events (
    id bigserial PRIMARY KEY,
    job_id uuid NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    attempt integer NOT NULL DEFAULT 0,
    event_type text NOT NULL CHECK(event_type IN ('state','progress','log')),
    level text NOT NULL DEFAULT 'info' CHECK(level IN ('debug','info','warn','error')),
    message text NOT NULL DEFAULT '',
    progress double precision CHECK(progress IS NULL OR (progress >= 0 AND progress <= 1)),
    data jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX job_events_job_id_id_idx ON job_events(job_id,id);
