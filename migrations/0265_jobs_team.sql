-- Apply before deploying readers of jobs.team (expansive: a metadata-only NOT NULL DEFAULT '').
-- The department or team the posting's ATS files it under, a plain trimmed string; '' when the
-- source states none. Not a taxonomy: the ATS's own label, verbatim.
ALTER TABLE public.jobs ADD COLUMN IF NOT EXISTS team text NOT NULL DEFAULT '';

COMMENT ON COLUMN public.jobs.team IS
    'Department or team the source ATS files the posting under, verbatim and trimmed; empty when unstated.';

-- A view's column list is fixed at creation: replace jobs_read so db.Job carries team. A column
-- can only be added at the END of a replaced view, so team follows the activity columns.
CREATE OR REPLACE VIEW jobs_read AS
SELECT
    j.id,
    j.source,
    j.external_id,
    j.url,
    j.title,
    j.company,
    j.location,
    j.remote,
    j.description,
    j.posted_at,
    j.created_at,
    j.updated_at,
    j.company_slug,
    j.enrichment,
    j.enriched_at,
    j.enrichment_version,
    j.public_slug,
    j.closed_at,
    j.countries,
    j.regions,
    j.work_mode,
    j.skills,
    j.seniority,
    j.category,
    j.created_by,
    j.updated_by,
    j.posting_language,
    j.employment_type,
    j.education_level,
    j.experience_years_min,
    j.collections,
    j.content_hash,
    j.english_level,
    j.cities,
    j.view_count,
    j.applied_count,
    j.role_fingerprint,
    j.semantic_embedded_model,
    j.semantic_embedded_hash,
    j.duplicate_of,
    j.is_tech,
    j.salary_min_manual,
    j.salary_max_manual,
    j.salary_currency_manual,
    j.salary_period_manual,
    j.upvote_count,
    j.downvote_count,
    j.ats_absent_at,
    j.closed_reason,
    j.is_private,
    j.salary_min_source,
    j.salary_max_source,
    j.salary_currency_source,
    j.salary_period_source,
    j.company_slug_folded,
    j.duplicate_of_aggregator,
    j.duplicate_of_role,
    j.duplicate_of_fuzzy,
    j.requires_clearance,
    j.requirements_derived,
    j.ai_interview_reports,
    j.metros,
    j.semantic_embedded_input_hash,
    j.via_recruiter,
    j.clearance_levels,
    j.equity,
    j.perks,
    j.application_deadline,
    j.engagement,
    a.last_seen_at,
    a.hydrated_at,
    COALESCE(a.liveness_strikes, 0)::integer AS liveness_strikes,
    j.team
FROM jobs j
-- Keep each lookup parameterized by the posting. A flattened LEFT JOIN can
-- merge-scan activity from its first ID for every page of a keyset scan.
LEFT JOIN LATERAL (
    SELECT x.last_seen_at, x.hydrated_at, x.liveness_strikes
    FROM job_activity x WHERE x.job_id = j.id
    LIMIT 1
) a ON true;

-- RefreshUnchangedJob now matches on team, so a team-only write must bump job_activity's
-- state_version too, or a refresh fenced on the old version would pass after a team change.
-- Same trigger as 0258 with team added.
DROP TRIGGER job_activity_state ON jobs;
CREATE TRIGGER job_activity_state AFTER UPDATE OF
    closed_at, content_hash, cities, salary_min_source, salary_max_source,
    salary_currency_source, salary_period_source, english_level, is_tech,
    engagement, application_deadline, duplicate_of, is_private,
    company_slug, posted_at, created_at,
    -- duplicate_of is derived by a BEFORE trigger from these owners. UPDATE OF
    -- checks the statement's SET list, including the owner rather than its result.
    duplicate_of_role, duplicate_of_aggregator, duplicate_of_fuzzy, team
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
    OLD.created_at IS DISTINCT FROM NEW.created_at OR
    OLD.team IS DISTINCT FROM NEW.team
) EXECUTE FUNCTION job_activity_state();
