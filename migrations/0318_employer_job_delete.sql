-- migrate: no-transaction
--
-- An employer can delete a vacancy it published, and sees the vacancies of its own company
-- that were collected from other sources.
--
-- Delete is soft: direct_applications and other tables reference the jobs row, so the row
-- stays. A deleted vacancy is closed with closed_reason 'employer_deleted'
-- and hidden from jobs_read, the read model behind the catalogue, company pages, the closed tab and
-- the job page, so none of them needs its own filter. The reason is distinct from
-- 'employer_closed', which ReopenEmployerJob can undo; this one is final. Readers of the
-- jobs table itself (applications, the employer's inbox) still see the row.
ALTER TABLE public.jobs
    DROP CONSTRAINT IF EXISTS jobs_closed_reason_check;

ALTER TABLE public.jobs
    ADD CONSTRAINT jobs_closed_reason_check CHECK (closed_reason = ANY (ARRAY[
        ''::text,
        'unseen'::text,
        'feed_removed'::text,
        'moderated'::text,
        'probe_expired'::text,
        'expired'::text,
        'board_unreachable'::text,
        'feed_empty'::text,
        'source_misattributed'::text,
        'employer_closed'::text,
        'board_retired'::text,
        'ats_covered'::text,
        'board_gone'::text,
        'native_source_pending'::text,
        'employer_deleted'::text
    ])) NOT VALID;

ALTER TABLE public.jobs
    VALIDATE CONSTRAINT jobs_closed_reason_check;

-- Same projection as 0274, minus the deleted vacancies. Column list unchanged.
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
    j.team,
    j.application_url
FROM jobs j
-- Keep each lookup parameterized by the posting. A flattened LEFT JOIN can
-- merge-scan activity from its first ID for every page of a keyset scan.
LEFT JOIN LATERAL (
    SELECT x.last_seen_at, x.hydrated_at, x.liveness_strikes
    FROM job_activity x WHERE x.job_id = j.id
    LIMIT 1
) a ON true
WHERE j.closed_reason <> 'employer_deleted';

-- The company an actor acts for, only while its account is active (confirmed): the ONE rule of
-- whose vacancies the employer surface may read, authored by the team or collected from an
-- ATS. NULL for everyone else, so `company_slug = active_employer_company(x)` matches nothing.
-- Reading is by company; writing stays by authorship (created_by in employer_team, source
-- 'employer'). Served by jobs_company_slug_idx.
CREATE OR REPLACE FUNCTION public.active_employer_company(actor bigint) RETURNS text
    LANGUAGE sql STABLE AS $$
    SELECT ca.company_slug FROM public.company_accounts ca
    WHERE ca.user_id = actor AND ca.status = 'active'
    UNION ALL
    SELECT ca.company_slug FROM public.company_account_members m
    JOIN public.company_accounts ca ON ca.company_slug = m.company_slug
    WHERE m.user_id = actor AND ca.status = 'active'
    LIMIT 1
$$;
