CREATE TYPE principal_role AS ENUM ('viewer','operator','admin');

CREATE TABLE principals (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL UNIQUE,
    role principal_role NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE api_keys (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    principal_id uuid NOT NULL REFERENCES principals(id) ON DELETE CASCADE,
    name text NOT NULL,
    key_hash bytea NOT NULL UNIQUE,
    prefix text NOT NULL,
    expires_at timestamptz,
    last_used_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(principal_id,name)
);

CREATE TABLE audit_log (
    id bigserial PRIMARY KEY,
    principal_id uuid REFERENCES principals(id) ON DELETE SET NULL,
    action text NOT NULL,
    resource_type text NOT NULL,
    resource_id text,
    method text NOT NULL,
    path text NOT NULL,
    status integer NOT NULL,
    remote_addr text,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_log_created_idx ON audit_log(created_at DESC,id DESC);

ALTER TABLE jobs ADD COLUMN owner_id uuid REFERENCES principals(id) ON DELETE SET NULL;
ALTER TABLE workflows ADD COLUMN owner_id uuid REFERENCES principals(id) ON DELETE SET NULL;
CREATE INDEX jobs_owner_idx ON jobs(owner_id,created_at DESC);
CREATE INDEX workflows_owner_idx ON workflows(owner_id,created_at DESC);

ALTER TABLE jobs ADD COLUMN retry_policy text NOT NULL DEFAULT 'exponential'
    CHECK(retry_policy IN ('fixed','linear','exponential'));
ALTER TABLE jobs ADD COLUMN retry_initial_seconds integer NOT NULL DEFAULT 2 CHECK(retry_initial_seconds > 0);
ALTER TABLE jobs ADD COLUMN retry_max_seconds integer NOT NULL DEFAULT 300 CHECK(retry_max_seconds > 0);
ALTER TABLE jobs ADD COLUMN retry_jitter boolean NOT NULL DEFAULT true;
ALTER TABLE jobs ADD COLUMN non_retryable_errors text[] NOT NULL DEFAULT '{}';

ALTER TABLE workflow_steps ADD COLUMN condition text;
ALTER TABLE workflow_steps ADD COLUMN continue_on_error boolean NOT NULL DEFAULT false;
ALTER TABLE workflow_steps ADD COLUMN output jsonb;
ALTER TYPE workflow_step_status ADD VALUE IF NOT EXISTS 'skipped';

-- Local development credentials. Only SHA-256 digests are stored.
WITH p AS (INSERT INTO principals(name,role) VALUES('local-admin','admin') RETURNING id)
INSERT INTO api_keys(principal_id,name,key_hash,prefix)
SELECT id,'development',digest('fq_admin_dev','sha256'),'fq_adm' FROM p;
WITH p AS (INSERT INTO principals(name,role) VALUES('local-operator','operator') RETURNING id)
INSERT INTO api_keys(principal_id,name,key_hash,prefix)
SELECT id,'development',digest('fq_operator_dev','sha256'),'fq_ope' FROM p;
WITH p AS (INSERT INTO principals(name,role) VALUES('local-viewer','viewer') RETURNING id)
INSERT INTO api_keys(principal_id,name,key_hash,prefix)
SELECT id,'development',digest('fq_viewer_dev','sha256'),'fq_vie' FROM p;
