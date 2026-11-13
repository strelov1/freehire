-- Release A: expand before deploying activity readers. Legacy writers remain valid.
-- The foreign key is added AFTER copying: otherwise every copied row locks jobs.
CREATE TABLE job_activity (
    job_id bigint PRIMARY KEY,
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    hydrated_at timestamptz,
    -- Consecutive probe failures, reset on success/reopen; retain the existing int32 contract.
    -- squawk-ignore prefer-bigint-over-int
    liveness_strikes integer NOT NULL DEFAULT 0,
    state_version bigint NOT NULL DEFAULT 0
) WITH (fillfactor = 80, autovacuum_vacuum_scale_factor = 0.02);

CREATE FUNCTION job_activity_insert() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO job_activity AS a (job_id, last_seen_at, hydrated_at, liveness_strikes)
    VALUES (NEW.id, NEW.last_seen_at, NEW.hydrated_at, NEW.liveness_strikes)
    -- During the pre-FK backfill window an explicit ID can reuse a deleted job.
    -- Replace its orphan observation instead of failing or inheriting stale state.
    ON CONFLICT (job_id) DO UPDATE SET
        last_seen_at = EXCLUDED.last_seen_at,
        hydrated_at = EXCLUDED.hydrated_at,
        liveness_strikes = EXCLUDED.liveness_strikes,
        state_version = a.state_version + 1;
    RETURN NULL;
END;
$$;
CREATE TRIGGER job_activity_insert AFTER INSERT ON jobs
    FOR EACH ROW EXECUTE FUNCTION job_activity_insert();

CREATE FUNCTION job_activity_legacy_bridge() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO job_activity AS a (job_id, last_seen_at, hydrated_at, liveness_strikes)
    VALUES (NEW.id, NEW.last_seen_at, NEW.hydrated_at, NEW.liveness_strikes)
    ON CONFLICT (job_id) DO UPDATE SET
        last_seen_at = GREATEST(a.last_seen_at, EXCLUDED.last_seen_at),
        hydrated_at = GREATEST(a.hydrated_at, EXCLUDED.hydrated_at),
        liveness_strikes = CASE
            WHEN OLD.liveness_strikes IS DISTINCT FROM NEW.liveness_strikes
            THEN EXCLUDED.liveness_strikes ELSE a.liveness_strikes END;
    RETURN NULL;
END;
$$;
CREATE TRIGGER job_activity_legacy_bridge
    AFTER UPDATE OF last_seen_at, hydrated_at, liveness_strikes ON jobs
    FOR EACH ROW WHEN (
        OLD.last_seen_at IS DISTINCT FROM NEW.last_seen_at OR
        OLD.hydrated_at IS DISTINCT FROM NEW.hydrated_at OR
        OLD.liveness_strikes IS DISTINCT FROM NEW.liveness_strikes
    ) EXECUTE FUNCTION job_activity_legacy_bridge();

-- Target-row version lets an activity-only UPDATE detect a content/lifecycle
-- change committed while it waited. No observation/counter write bumps it.
CREATE FUNCTION job_activity_state() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    UPDATE job_activity SET
        state_version = state_version + 1,
        liveness_strikes = CASE
            WHEN OLD.closed_at IS NOT NULL AND NEW.closed_at IS NULL THEN 0
            ELSE liveness_strikes END
    WHERE job_id = NEW.id;
    RETURN NULL;
END;
$$;
CREATE TRIGGER job_activity_state AFTER UPDATE OF
    closed_at, content_hash, cities, salary_min_source, salary_max_source,
    salary_currency_source, salary_period_source, english_level, is_tech,
    engagement, application_deadline, duplicate_of, is_private,
    company_slug, posted_at, created_at,
    -- duplicate_of is derived by a BEFORE trigger from these owners. UPDATE OF
    -- checks the statement's SET list, including the owner rather than its result.
    duplicate_of_role, duplicate_of_aggregator, duplicate_of_fuzzy
ON jobs FOR EACH ROW WHEN (
    OLD.closed_at IS DISTINCT FROM NEW.closed_at OR
    OLD.content_hash IS DISTINCT FROM NEW.content_hash OR
    OLD.cities IS DISTINCT FROM NEW.cities OR
    OLD.salary_min_source IS DISTINCT FROM NEW.salary_min_source OR
    OLD.salary_max_source IS DISTINCT FROM NEW.salary_max_source OR
    OLD.salary_currency_source IS DISTINCT FROM NEW.salary_currency_source OR
    OLD.salary_period_source IS DISTINCT FROM NEW.salary_period_source OR
    OLD.english_level IS DISTINCT FROM NEW.english_level OR
    OLD.is_tech IS DISTINCT FROM NEW.is_tech OR
    OLD.engagement IS DISTINCT FROM NEW.engagement OR
    OLD.application_deadline IS DISTINCT FROM NEW.application_deadline OR
    OLD.duplicate_of IS DISTINCT FROM NEW.duplicate_of OR
    OLD.is_private IS DISTINCT FROM NEW.is_private OR
    OLD.company_slug IS DISTINCT FROM NEW.company_slug OR
    OLD.posted_at IS DISTINCT FROM NEW.posted_at OR
    OLD.created_at IS DISTINCT FROM NEW.created_at
) EXECUTE FUNCTION job_activity_state();
