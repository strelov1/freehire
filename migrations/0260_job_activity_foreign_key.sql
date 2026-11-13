-- Release A: record the completed copy before taking the brief parent DDL lock.
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM jobs j WHERE NOT EXISTS (
        SELECT 1 FROM job_activity a WHERE a.job_id = j.id
    )) THEN
        RAISE EXCEPTION 'job_activity backfill incomplete';
    END IF;
END $$;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint
                   WHERE conrelid = 'job_activity'::regclass
                     AND conname = 'job_activity_job_id_fkey') THEN
        ALTER TABLE job_activity ADD CONSTRAINT job_activity_job_id_fkey
            FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE NOT VALID;
    END IF;
END $$;

