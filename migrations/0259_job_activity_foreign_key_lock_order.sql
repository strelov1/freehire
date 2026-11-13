-- Release A: take the parent DDL lock before the child, matching bridge writers.
-- ADD FOREIGN KEY alone takes job_activity first, then jobs: that deadlocks with
-- a concurrent jobs update whose legacy bridge needs job_activity (40P01).
-- This file sorts after the recorded copy and before immutable 0260; the latter
-- keeps its completeness check and skips ADD once this constraint exists.
-- Keep LOCK inside DO so this also works when initdb runs the file in autocommit.
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint
                   WHERE conrelid = 'job_activity'::regclass
                     AND conname = 'job_activity_job_id_fkey') THEN
        LOCK TABLE jobs IN SHARE ROW EXCLUSIVE MODE;
        ALTER TABLE job_activity ADD CONSTRAINT job_activity_job_id_fkey
            FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE NOT VALID;
    END IF;
END $$;
