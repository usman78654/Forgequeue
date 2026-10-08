ALTER TABLE jobs ADD COLUMN cancellation_requested_at timestamptz;
ALTER TABLE jobs ADD COLUMN cancellation_reason text;
CREATE INDEX jobs_cancelling_idx ON jobs(cancellation_requested_at) WHERE status = 'cancelling';
