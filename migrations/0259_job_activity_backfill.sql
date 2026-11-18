-- migrate: no-transaction
-- Release A: each bounded range commits independently. Triggers installed by
-- 0258 preserve concurrent inserts/updates; rerunning never overwrites them.
DO $$
DECLARE
    last_id bigint := 0;
    end_id bigint;
    upper_id bigint;
BEGIN
    SELECT max(id) INTO upper_id FROM jobs;
    WHILE last_id < upper_id LOOP
        end_id := LEAST(last_id + 10000, upper_id);
        INSERT INTO job_activity (job_id, last_seen_at, hydrated_at, liveness_strikes)
        SELECT id, last_seen_at, hydrated_at, liveness_strikes FROM jobs
        WHERE id > last_id AND id <= end_id
        ON CONFLICT (job_id) DO NOTHING;
        last_id := end_id;
        COMMIT;
    END LOOP;
END;
$$;

ANALYZE job_activity;

