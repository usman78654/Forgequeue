ALTER TABLE jobs ADD COLUMN lease_token bigint NOT NULL DEFAULT 0;
ALTER TABLE job_attempts ADD COLUMN lease_token bigint NOT NULL DEFAULT 0;
CREATE INDEX jobs_active_lease_token_idx ON jobs(id, lease_token) WHERE status = 'running';
