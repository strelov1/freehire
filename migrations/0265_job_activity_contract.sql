-- Release C: deploy only after every reader and writer is B-compatible, including
-- private operations consumers. Wait for long catalogue scans before this DDL.
-- B binaries are the rollback floor; a legacy binary cannot run after contraction.
-- Dropping columns does not reclaim the existing jobs heap or prove disk savings.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'job_activity'::regclass
          AND conname = 'job_activity_job_id_fkey' AND convalidated
    ) THEN
        RAISE EXCEPTION 'job_activity must have a validated foreign key before contraction';
    END IF;
END;
$$;

-- Raw inserts still establish activity. Hydration is recorded separately by
-- ObserveJobActivity after an intake transaction has actually read the body.
CREATE OR REPLACE FUNCTION job_activity_insert() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO job_activity AS a (job_id) VALUES (NEW.id)
    ON CONFLICT (job_id) DO UPDATE SET
        last_seen_at = now(), hydrated_at = NULL, liveness_strikes = 0,
        state_version = a.state_version + 1;
    RETURN NULL;
END;
$$;

DROP TRIGGER job_activity_legacy_bridge ON jobs;
DROP FUNCTION job_activity_legacy_bridge();
-- B-compatible binaries and audited operations no longer name these columns.
-- The deploy gate verifies the legacy process drain before applying this contract.
ALTER TABLE jobs
    -- squawk-ignore ban-drop-column
    DROP COLUMN last_seen_at,
    -- squawk-ignore ban-drop-column
    DROP COLUMN hydrated_at,
    -- squawk-ignore ban-drop-column
    DROP COLUMN liveness_strikes;
