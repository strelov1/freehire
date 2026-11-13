-- name: UpsertManualJob :one
-- Moderator-authored write: the hand-curated analogue of UpsertJob. source is the
-- posting's real origin (e.g. 'workatastartup'), supplied by the moderator and
-- defaulting to 'manual'; the dedup key is (source, external_id = url), so re-POSTing
-- the same URL updates the row idempotently instead of duplicating it. The manual
-- provenance is recorded by created_by (set here, NULL for every automated source) —
-- not by the source value. created_by is stamped once at insert; updated_by is
-- (re)written on the conflict update. Like UpsertJob, public_slug is minted once and
-- never rewritten, and the enrichment columns are otherwise left to SetJobEnrichment —
-- the one exception is an authoritative manual salary, which is written to the
-- salary_*_manual columns AND seeded into the enrichment payload here so the vacancy
-- shows its salary immediately, before any enrichment pass runs (the pass then preserves
-- it via SetJobEnrichment's overlay). The conflict reopens a previously closed posting
-- (closed_at = NULL) since the moderator is re-asserting it.
WITH company_upsert AS (
    INSERT INTO companies (slug, name)
    SELECT sqlc.arg(company_slug), sqlc.arg(company)
    WHERE sqlc.arg(company_slug) <> ''
    ON CONFLICT (slug) DO UPDATE SET
        name       = EXCLUDED.name,
        updated_at = now()
)
INSERT INTO jobs (
    source, external_id, url, title, company, company_slug, company_slug_folded, location, remote, description, posted_at,
    public_slug, countries, regions, cities, metros, work_mode, skills, seniority, category, is_tech, requires_clearance, via_recruiter, equity, clearance_levels, perks, engagement,
    posting_language, employment_type, education_level, english_level, experience_years_min,
    salary_min_manual, salary_max_manual, salary_currency_manual, salary_period_manual, enrichment,
    content_hash, role_fingerprint, requirements_derived,
    created_by
) VALUES (
    sqlc.arg(source), sqlc.arg(external_id), sqlc.arg(url), sqlc.arg(title),
    sqlc.arg(company), sqlc.arg(company_slug), replace(sqlc.arg(company_slug), '-', ''), sqlc.arg(location), sqlc.arg(remote),
    sqlc.arg(description), sqlc.arg(posted_at),
    sqlc.arg(public_slug),
    COALESCE(sqlc.arg(countries)::text[], '{}'), COALESCE(sqlc.arg(regions)::text[], '{}'), COALESCE(sqlc.arg(cities)::text[], '{}'), COALESCE(sqlc.arg(metros)::text[], '{}'),
    sqlc.arg(work_mode), COALESCE(sqlc.arg(skills)::text[], '{}'),
    sqlc.arg(seniority), sqlc.arg(category), sqlc.arg(is_tech), sqlc.arg(requires_clearance), sqlc.arg(via_recruiter), sqlc.arg(equity), COALESCE(sqlc.arg(clearance_levels)::text[], '{}'), COALESCE(sqlc.arg(perks)::text[], '{}'), COALESCE(sqlc.arg(engagement)::text[], '{}'),
    sqlc.arg(posting_language), sqlc.arg(employment_type), sqlc.arg(education_level), sqlc.arg(english_level), sqlc.arg(experience_years_min),
    sqlc.arg(salary_min_manual), sqlc.arg(salary_max_manual), sqlc.arg(salary_currency_manual), sqlc.arg(salary_period_manual),
    -- Seed the enrichment salary from the manual salary so it displays before any pass;
    -- '{}' when no bound is stated (the presence signal), leaving enrichment empty.
    CASE
        WHEN sqlc.arg(salary_min_manual)::int IS NOT NULL OR sqlc.arg(salary_max_manual)::int IS NOT NULL
        THEN jsonb_strip_nulls(jsonb_build_object(
            'salary_min', sqlc.arg(salary_min_manual)::int,
            'salary_max', sqlc.arg(salary_max_manual)::int,
            'salary_currency', NULLIF(sqlc.arg(salary_currency_manual), ''),
            'salary_period', NULLIF(sqlc.arg(salary_period_manual), '')
        ))
        ELSE '{}'::jsonb
    END,
    sqlc.arg(content_hash), sqlc.arg(role_fingerprint),
    -- Derived from the moderator's own description by the same mapping the crawl uses
    -- (internal/job/job's withDerived), so a hand-curated posting is not the one kind
    -- of posting whose requirements never appear.
    COALESCE(sqlc.arg(requirements_derived)::jsonb, '[]'::jsonb),
    sqlc.arg(created_by)::bigint
)
ON CONFLICT (source, external_id) DO UPDATE SET
    url          = EXCLUDED.url,
    title        = EXCLUDED.title,
    company      = EXCLUDED.company,
    company_slug = EXCLUDED.company_slug,
    company_slug_folded = EXCLUDED.company_slug_folded,
    location     = EXCLUDED.location,
    remote       = EXCLUDED.remote,
    description  = EXCLUDED.description,
    posted_at    = EXCLUDED.posted_at,
    countries    = EXCLUDED.countries,
    regions      = EXCLUDED.regions,
    cities       = EXCLUDED.cities,
    metros       = EXCLUDED.metros,
    work_mode    = EXCLUDED.work_mode,
    skills       = EXCLUDED.skills,
    seniority    = EXCLUDED.seniority,
    category     = EXCLUDED.category,
    is_tech      = EXCLUDED.is_tech,
    requires_clearance = EXCLUDED.requires_clearance,
    clearance_levels   = EXCLUDED.clearance_levels,
    -- The moderator's description replaces the stored one unconditionally, so the
    -- mark is re-derived from it unconditionally too.
    via_recruiter = EXCLUDED.via_recruiter,
    equity = EXCLUDED.equity,
    perks        = EXCLUDED.perks,
    engagement   = EXCLUDED.engagement,
    posting_language     = EXCLUDED.posting_language,
    employment_type      = EXCLUDED.employment_type,
    education_level      = EXCLUDED.education_level,
    english_level        = EXCLUDED.english_level,
    experience_years_min = EXCLUDED.experience_years_min,
    salary_min_manual      = EXCLUDED.salary_min_manual,
    salary_max_manual      = EXCLUDED.salary_max_manual,
    salary_currency_manual = EXCLUDED.salary_currency_manual,
    salary_period_manual   = EXCLUDED.salary_period_manual,
    -- A re-create rewrites the content, so both derived columns move with it (see
    -- UpsertJob): content_hash is what makes a later edit re-embed, role_fingerprint
    -- what lets a hand-curated posting cluster with the crawled copy of its role.
    content_hash     = EXCLUDED.content_hash,
    role_fingerprint = EXCLUDED.role_fingerprint,
    -- Same rule the crawl path uses, and keyed on the same signal: an empty incoming
    -- description means the write carries no content to derive from, anything else
    -- means the derivation is current — including when it is empty because the edit
    -- removed the section.
    requirements_derived = CASE
        WHEN EXCLUDED.description <> '' THEN EXCLUDED.requirements_derived
        ELSE jobs.requirements_derived
    END,
    -- Overlay the (possibly changed) manual salary onto the existing enrichment so a
    -- re-create reflects it immediately while preserving any prior LLM enrichment.
    enrichment = CASE
        WHEN EXCLUDED.salary_min_manual IS NOT NULL OR EXCLUDED.salary_max_manual IS NOT NULL
        THEN jobs.enrichment || jsonb_strip_nulls(jsonb_build_object(
            'salary_min', EXCLUDED.salary_min_manual,
            'salary_max', EXCLUDED.salary_max_manual,
            'salary_currency', NULLIF(EXCLUDED.salary_currency_manual, ''),
            'salary_period', NULLIF(EXCLUDED.salary_period_manual, '')
        ))
        ELSE jobs.enrichment
    END,
    updated_by   = sqlc.arg(updated_by)::bigint,
    -- A moderator re-create reopens the job; reset the strike count too so the
    -- two-strike liveness grace survives a reopen (see UpsertJob).
    closed_at    = NULL,
    closed_reason = '',
    liveness_strikes = CASE WHEN jobs.closed_at IS NOT NULL THEN 0 ELSE jobs.liveness_strikes END,
    updated_at   = now()
RETURNING jobs.id;

-- name: InsertPrivateJob :one
-- Creates a job visible only to its creator: the jd-tailor-intake private-JD path
-- (pasted text, or a URL only a generic scrape could read). Always a plain INSERT,
-- never an upsert — external_id is a synthetic value scoped to this one submission
-- (see internal/job/privatejob), never compared against the public (source, external_id)
-- dedup space, so two submissions never collide and this never conflicts with an
-- existing row.
--
-- Deliberately does NOT touch the companies table (unlike UpsertJob/UpsertManualJob):
-- a private submission's employer name is not a vetted catalogue entry, so minting or
-- updating a companies row from it would leak a one-off private JD's company into the
-- public companies directory. jobs.company_slug has no FK to companies, so this is
-- safe to leave unbacked.
--
-- Also deliberately does NOT enqueue enrichment (contrast UpsertManualJob's Repository,
-- which does): a private, single-tailoring-session row doesn't recoup that cost. The
-- caller supplies content_hash/role_fingerprint precomputed the same way every other
-- write path does (job.Fields.UpsertParams), so a private job's fingerprints are
-- comparable if it were ever to matter, even though it is never indexed or clustered.
INSERT INTO jobs (
    source, external_id, url, title, company, company_slug, company_slug_folded, location, remote, description,
    public_slug, countries, regions, cities, metros, work_mode, skills, seniority, category, is_tech, requires_clearance, via_recruiter, equity, clearance_levels, perks, engagement,
    posting_language, employment_type, education_level, english_level, experience_years_min,
    content_hash, role_fingerprint, requirements_derived,
    created_by, is_private
) VALUES (
    sqlc.arg(source), sqlc.arg(external_id), sqlc.arg(url), sqlc.arg(title),
    sqlc.arg(company), sqlc.arg(company_slug), replace(sqlc.arg(company_slug), '-', ''), sqlc.arg(location), sqlc.arg(remote),
    sqlc.arg(description),
    sqlc.arg(public_slug),
    COALESCE(sqlc.arg(countries)::text[], '{}'), COALESCE(sqlc.arg(regions)::text[], '{}'), COALESCE(sqlc.arg(cities)::text[], '{}'), COALESCE(sqlc.arg(metros)::text[], '{}'),
    sqlc.arg(work_mode), COALESCE(sqlc.arg(skills)::text[], '{}'),
    sqlc.arg(seniority), sqlc.arg(category), sqlc.arg(is_tech), sqlc.arg(requires_clearance), sqlc.arg(via_recruiter), sqlc.arg(equity), COALESCE(sqlc.arg(clearance_levels)::text[], '{}'), COALESCE(sqlc.arg(perks)::text[], '{}'), COALESCE(sqlc.arg(engagement)::text[], '{}'),
    sqlc.arg(posting_language), sqlc.arg(employment_type), sqlc.arg(education_level), sqlc.arg(english_level), sqlc.arg(experience_years_min),
    sqlc.arg(content_hash), sqlc.arg(role_fingerprint),
    -- A pasted private JD is exactly the text the tailor reads requirements out of,
    -- so it derives them like every other write path.
    COALESCE(sqlc.arg(requirements_derived)::jsonb, '[]'::jsonb),
    sqlc.arg(created_by)::bigint, true
)
RETURNING jobs.id;

-- name: UpdateManualJob :one
-- Moderator edit of a hand-curated job, addressed by public_slug and scoped to
-- created_by IS NOT NULL so this path can only rewrite a moderator-authored posting,
-- never an automated-source (ingest/telegram) one — regardless of the declared source.
-- AND NOT is_private is the second half of that scope, not a duplicate of it:
-- InsertPrivateJob stamps created_by too, so created_by IS NOT NULL alone also matched
-- every jd-tailor-intake private JD, and a moderator holding the slug could rewrite one
-- user's private text. The refusal in moderation.QueriesRepository.BySlug is what stops
-- the service path reaching this statement at all — the company_upsert CTE below is
-- independent of the UPDATE and would mint a public companies row from a private JD's
-- employer even on a zero-row update, which is exactly what InsertPrivateJob refuses to
-- do. This clause is the guard in the statement for any caller that arrives without it.
-- The partial merge (nil = unchanged) and facet re-derivation happen in the service; this
-- query writes the resulting full field set, so geography/skills/company_slug stay
-- consistent with the edited content. The source identity (url/external_id/public_slug)
-- is deliberately NOT updatable here. The company row is upserted when a slug is present,
-- so "a company's jobs" stays resolvable. updated_by records the acting moderator. Returns
-- no row when the slug is missing or not a manual job (the caller maps that to 404).
-- closed_at is deliberately NOT touched: an edit is a content fix, not a lifecycle change.
-- Reopening a closed posting is the re-create (same-URL UpsertManualJob) path's job, so a
-- content edit never resurrects a job the sweep/liveness worker closed.
WITH company_upsert AS (
    INSERT INTO companies (slug, name)
    SELECT sqlc.arg(company_slug), sqlc.arg(company)
    WHERE sqlc.arg(company_slug) <> ''
    ON CONFLICT (slug) DO UPDATE SET
        name       = EXCLUDED.name,
        updated_at = now()
)
UPDATE jobs
SET title        = sqlc.arg(title),
    company      = sqlc.arg(company),
    company_slug = sqlc.arg(company_slug),
    company_slug_folded = replace(sqlc.arg(company_slug), '-', ''),
    location     = sqlc.arg(location),
    remote       = sqlc.arg(remote),
    description  = sqlc.arg(description),
    posted_at    = sqlc.arg(posted_at),
    countries    = COALESCE(sqlc.arg(countries)::text[], '{}'),
    regions      = COALESCE(sqlc.arg(regions)::text[], '{}'),
    cities       = COALESCE(sqlc.arg(cities)::text[], '{}'),
    metros       = COALESCE(sqlc.arg(metros)::text[], '{}'),
    work_mode    = sqlc.arg(work_mode),
    skills       = COALESCE(sqlc.arg(skills)::text[], '{}'),
    seniority    = sqlc.arg(seniority),
    category     = sqlc.arg(category),
    is_tech      = sqlc.arg(is_tech),
    requires_clearance = sqlc.arg(requires_clearance),
    clearance_levels   = COALESCE(sqlc.arg(clearance_levels)::text[], '{}'),
    via_recruiter = sqlc.arg(via_recruiter),
    equity = sqlc.arg(equity),
    perks        = COALESCE(sqlc.arg(perks)::text[], '{}'),
    engagement   = COALESCE(sqlc.arg(engagement)::text[], '{}'),
    posting_language     = sqlc.arg(posting_language),
    employment_type      = sqlc.arg(employment_type),
    education_level      = sqlc.arg(education_level),
    english_level        = sqlc.arg(english_level),
    experience_years_min = sqlc.arg(experience_years_min),
    -- The edit re-derives the facets from the edited content, so both derived columns
    -- move with it. content_hash is what makes the edit re-embed at all: the trigger is
    -- `semantic_embedded_hash IS DISTINCT FROM content_hash`, so leaving the stored hash
    -- behind would freeze the vector on the pre-edit text.
    content_hash     = sqlc.arg(content_hash),
    role_fingerprint = sqlc.arg(role_fingerprint),
    -- Re-derived from the edited description too, and written unconditionally rather
    -- than with the crawl path's keep-the-stored-value guard: a moderator DELETING a
    -- requirements list from a posting means it no longer has one, unlike a detail
    -- fetch that failed.
    requirements_derived = COALESCE(sqlc.arg(requirements_derived)::jsonb, '[]'::jsonb),
    updated_by   = sqlc.arg(updated_by)::bigint,
    updated_at   = now()
WHERE public_slug = sqlc.arg(public_slug) AND created_by IS NOT NULL AND NOT is_private
RETURNING jobs.id;

-- name: UpdateEmployerJob :one
-- The employer-authored analogue of UpdateManualJob, scoped narrower on purpose: WHERE
-- created_by = actor_id (not merely IS NOT NULL) AND source = 'employer'. UpdateManualJob's
-- own scope lets any moderator edit any manually-authored job, which is correct for trusted
-- staff and wrong for a self-service employer — see openspec/changes/
-- add-employer-company-accounts/design.md for why this needed its own query rather than a
-- widened moderation one. company_slug/company are always the caller's locked company
-- identity (internal/ingest/employer never lets them vary per edit), so company_upsert here
-- only ever re-affirms the same row UpdateManualJob's would. closed_at is deliberately NOT
-- touched, matching UpdateManualJob: an edit is a content fix, not a lifecycle change.
WITH company_upsert AS (
    INSERT INTO companies (slug, name)
    SELECT sqlc.arg(company_slug), sqlc.arg(company)
    WHERE sqlc.arg(company_slug) <> ''
    ON CONFLICT (slug) DO UPDATE SET
        name       = EXCLUDED.name,
        updated_at = now()
)
UPDATE jobs
SET title        = sqlc.arg(title),
    company      = sqlc.arg(company),
    company_slug = sqlc.arg(company_slug),
    company_slug_folded = replace(sqlc.arg(company_slug), '-', ''),
    location     = sqlc.arg(location),
    remote       = sqlc.arg(remote),
    description  = sqlc.arg(description),
    posted_at    = sqlc.arg(posted_at),
    countries    = COALESCE(sqlc.arg(countries)::text[], '{}'),
    regions      = COALESCE(sqlc.arg(regions)::text[], '{}'),
    cities       = COALESCE(sqlc.arg(cities)::text[], '{}'),
    metros       = COALESCE(sqlc.arg(metros)::text[], '{}'),
    work_mode    = sqlc.arg(work_mode),
    skills       = COALESCE(sqlc.arg(skills)::text[], '{}'),
    seniority    = sqlc.arg(seniority),
    category     = sqlc.arg(category),
    is_tech      = sqlc.arg(is_tech),
    requires_clearance = sqlc.arg(requires_clearance),
    clearance_levels   = COALESCE(sqlc.arg(clearance_levels)::text[], '{}'),
    via_recruiter = sqlc.arg(via_recruiter),
    equity = sqlc.arg(equity),
    perks        = COALESCE(sqlc.arg(perks)::text[], '{}'),
    engagement   = COALESCE(sqlc.arg(engagement)::text[], '{}'),
    posting_language     = sqlc.arg(posting_language),
    employment_type      = sqlc.arg(employment_type),
    education_level      = sqlc.arg(education_level),
    english_level        = sqlc.arg(english_level),
    experience_years_min = sqlc.arg(experience_years_min),
    content_hash     = sqlc.arg(content_hash),
    role_fingerprint = sqlc.arg(role_fingerprint),
    requirements_derived = COALESCE(sqlc.arg(requirements_derived)::jsonb, '[]'::jsonb),
    updated_by   = sqlc.arg(updated_by)::bigint,
    updated_at   = now()
WHERE public_slug = sqlc.arg(public_slug)
  AND created_by = sqlc.arg(actor_id)::bigint
  AND source = 'employer'
  AND NOT is_private
RETURNING jobs.id;

-- name: ListEmployerJobs :many
-- An employer's own dashboard list: every vacancy this actor created through this path,
-- newest first, open and closed both (the dashboard shows a closed one with its own badge
-- rather than dropping it — the employer's own re-Create-to-reopen path needs the closed
-- ones findable). Capped rather than paginated: an MVP dashboard for one employer's own
-- postings, not the public catalogue.
SELECT *
FROM jobs_read
WHERE created_by = sqlc.arg(actor_id)::bigint
  AND source = 'employer'
ORDER BY created_at DESC
LIMIT 200;

-- name: CloseEmployerJob :one
-- Soft-close, the employer-self-service analogue of CloseJobByID: scoped to the actor's own
-- employer-authored posting (created_by = actor_id AND source = 'employer'), never any other
-- job. closed_reason = 'employer_closed' (migration 0175) keeps this mechanism distinguishable
-- from a moderator's 'moderated' close — a different actor and a different trust level.
-- WHERE closed_at IS NULL keeps it idempotent, matching every other closer.
WITH closed AS (
    UPDATE jobs
    SET closed_at     = now(),
        closed_reason = 'employer_closed',
        updated_at    = now()
    WHERE jobs.public_slug = sqlc.arg(public_slug)
      AND jobs.created_by = sqlc.arg(actor_id)::bigint
      AND jobs.source = 'employer'
      AND jobs.closed_at IS NULL
    RETURNING jobs.id
), queued AS (
    INSERT INTO search_delete_outbox (job_id)
    SELECT id FROM closed
    ON CONFLICT (job_id) DO NOTHING
)
SELECT count(*) FROM closed;

-- name: ReopenEmployerJob :one
-- Undoes CloseEmployerJob: the same actor, the same scope (created_by = actor_id AND
-- source = 'employer'), and ONLY a close that actor made — closed_reason =
-- 'employer_closed'. A posting another mechanism closed (a moderator, the liveness probe)
-- stays closed; reopening it is not the employer's call. A vacancy taking applications on
-- freehire has no URL to re-create under, so without this its employer could never undo a
-- close. The search index learns of it twice over: a pending delete is dropped (left in
-- place, the drain would remove the reopened row after re-adding it) and the row is queued
-- for the facet index. Returns the rows reopened, 0 when there was nothing to undo.
WITH reopened AS (
    UPDATE jobs
    SET closed_at     = NULL,
        closed_reason = '',
        updated_at    = now()
    WHERE jobs.public_slug = sqlc.arg(public_slug)
      AND jobs.created_by = sqlc.arg(actor_id)::bigint
      AND jobs.source = 'employer'
      AND jobs.closed_reason = 'employer_closed'
    RETURNING jobs.id, COALESCE(jobs.posted_at, jobs.created_at) AS posted
), undeleted AS (
    DELETE FROM search_delete_outbox d
    USING reopened
    WHERE d.job_id = reopened.id
), queued AS (
    INSERT INTO search_outbox (job_id, job_posted_at)
    SELECT id, posted FROM reopened
    ON CONFLICT (job_id) DO NOTHING
)
SELECT count(*) FROM reopened;
