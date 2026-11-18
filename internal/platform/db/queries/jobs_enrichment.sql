-- name: EnqueueJobEnrichment :execrows
-- Transactional-outbox enqueue for the ingest write path: queue this one job for
-- enrichment, gated on the same conditions the backfill uses (unenriched or below the
-- target schema version, and confirmed technical), so an already-enriched job is not
-- re-queued and LLM budget is spent only where jobderive.deriveIsTech already found
-- technical evidence (is_tech = true) — never a confirmed non-tech role (is_tech =
-- false) and, deliberately, never an unresolved one either (is_tech IS NULL: neither
-- the title dictionary nor the description found tech OR non-tech evidence). That
-- unresolved bucket used to enqueue by default — the earlier reasoning was "never
-- silently skip a tech job the dictionary missed" — but measured at catalogue scale it
-- was ~65% of the open catalogue and enrichment returned nothing useful for ~91% of it
-- (broad multi-industry ATS crawls: painters, stockers, drivers), so the LLM spend was
-- not buying the coverage it cost. Also requires a non-empty description: the LLM has
-- nothing to extract from a blank one regardless of category, and a 2026-08-06 prod
-- sweep found ~53K such rows already sitting in the queue for no reason. Also requires
-- duplicate_of IS NULL, matching EnqueuePendingJobs: the write path's own dedup only
-- sets duplicate_of on the INSERT that first clusters a repost, so a later recrawl of
-- an already-deduped job reaches this call as an UPDATE and would otherwise queue a
-- row ClaimEnrichmentBatch's duplicate_of IS NULL filter can never claim. Idempotent
-- via the outbox's UNIQUE (job_id, target_version). Run in the same transaction as the
-- job's UpsertJob so a newly ingested job is queued atomically with its write.
INSERT INTO enrichment_outbox (job_id, target_version)
SELECT id, sqlc.arg(target_version)::int
FROM jobs
WHERE id = sqlc.arg(job_id)::bigint
  AND (enriched_at IS NULL OR enrichment_version < sqlc.arg(target_version)::int)
  AND is_tech IS TRUE
  AND description <> ''
  AND duplicate_of IS NULL
ON CONFLICT (job_id, target_version) DO NOTHING;

-- name: SetJobEnrichment :exec
-- Targeted enrichment write used by the enrichment command: set only the payload
-- and the provenance stamp, touching no raw source field. Kept separate from
-- UpsertJob (the ingest full-upsert path) so ingest and enrichment stay decoupled.
-- Two salary overlays chain over the incoming LLM payload via jsonb `||` (later wins),
-- so the effective precedence is manual > source > LLM-guessed:
--   1. salary_*_source: the ATS's own structured salary (Lever/Ashby/Recruitee — see
--      migration 0093). The LLM can still compute its own figure for a job without one,
--      but a structured value is never worse than a guess, so it wins when present.
--   2. salary_*_manual: an authoritative manual salary a recruiter/moderator stated by
--      hand (migration 0031) — wins over both, since a human confirmed it.
-- jsonb_strip_nulls drops an unstated bound so an overlay firing on just one of
-- min/max does not blank the other's payload value; each overlay only fires at all
-- when at least one of its own bounds is set (the presence signal).
--
-- There is deliberately NO overlay for requirements_derived here, though one lived in
-- this statement briefly. It would MATERIALISE the derived list into the blob, and
-- nothing revises a blob: a later crawl rewrites the column and leaves the copy, so a
-- description edit that deleted the requirements section would leave the page quoting
-- a posting that no longer says it — and the backfill could not reach it either. The
-- fold belongs on the read path, where it re-reads the column every time
-- (jobview.FromDomain). The column stays the single source; the blob holds only what
-- the model itself said.
UPDATE jobs
SET enrichment         =
        sqlc.arg(enrichment)::jsonb
        || CASE
            WHEN salary_min_source IS NOT NULL OR salary_max_source IS NOT NULL
            THEN jsonb_strip_nulls(jsonb_build_object(
                'salary_min', salary_min_source,
                'salary_max', salary_max_source,
                'salary_currency', NULLIF(salary_currency_source, ''),
                'salary_period', NULLIF(salary_period_source, '')
            ))
            ELSE '{}'::jsonb
        END
        || CASE
            WHEN salary_min_manual IS NOT NULL OR salary_max_manual IS NOT NULL
            THEN jsonb_strip_nulls(jsonb_build_object(
                'salary_min', salary_min_manual,
                'salary_max', salary_max_manual,
                'salary_currency', NULLIF(salary_currency_manual, ''),
                'salary_period', NULLIF(salary_period_manual, '')
            ))
            ELSE '{}'::jsonb
        END,
    enriched_at        = sqlc.arg(enriched_at),
    enrichment_version = sqlc.arg(enrichment_version),
    updated_at         = now()
WHERE id = sqlc.arg(id);

-- name: UpdateJobDerived :exec
-- One-off re-derive (cmd/backfill-derive): rewrite in a single pass every column that
-- ingest computes as a pure function of a row's own raw/immutable fields — the
-- deterministic dictionary facets (countries, regions, cities, work_mode, skills,
-- seniority, category, is_tech, requires_clearance, via_recruiter, equity, clearance_levels, perks, engagement, requirements_derived (reqextract), plus the synthetic enrichment
-- facets posting_language,
-- employment_type, education_level, english_level, experience_years_min, all from
-- jobderive.Derive), the repost-identity role_fingerprint (internal/job/jobhash), and the
-- company_slug (internal/dict/normalize). public_slug is deliberately NOT here, for the
-- reason UpsertJob gives: it is a published URL, minted once, and a re-derive from a title
-- the source has since changed would turn every link to it into a 404 (freehire#3284).
-- One keyset scan propagates any
-- dictionary/algorithm change to old and closed rows that never re-crawl. Every column
-- is a pure function of the raw fields, so the write is idempotent. COALESCE maps a nil
-- array arg to '{}' for the NOT NULL array columns; work_mode is written as given by the
-- caller, which preserves an already-set (possibly adapter-structured) value.
-- employment_type is likewise the caller's: a stored type is kept, except a part_time
-- the text parser now attributes only to a pay-policy sentence (jobderive.RetainEmploymentType).
--
-- updated_at is bumped ONLY when role_fingerprint actually moves (the SET clause reads
-- the pre-update row, so the guard compares the stored fingerprint to the new one): a
-- fingerprint change must reach `reindex --since`, which recomputes duplicate_of, while a
-- facet/company-slug-only rewrite deliberately leaves the timestamp untouched so a big backfill
-- does not churn every row's updated_at.
UPDATE jobs
SET countries = COALESCE(sqlc.arg(countries)::text[], '{}'),
    regions   = COALESCE(sqlc.arg(regions)::text[], '{}'),
    cities    = COALESCE(sqlc.arg(cities)::text[], '{}'),
    metros    = COALESCE(sqlc.arg(metros)::text[], '{}'),
    work_mode = sqlc.arg(work_mode),
    skills    = COALESCE(sqlc.arg(skills)::text[], '{}'),
    seniority = sqlc.arg(seniority),
    category  = sqlc.arg(category),
    is_tech   = sqlc.arg(is_tech),
    requires_clearance = sqlc.arg(requires_clearance),
    clearance_levels   = COALESCE(sqlc.arg(clearance_levels)::text[], '{}'),
    via_recruiter = sqlc.arg(via_recruiter),
    equity = sqlc.arg(equity),
    perks        = COALESCE(sqlc.arg(perks)::text[], '{}'),
    engagement   = COALESCE(sqlc.arg(engagement)::text[], '{}'),
    requirements_derived = COALESCE(sqlc.arg(requirements_derived)::jsonb, '[]'::jsonb),
    posting_language     = sqlc.arg(posting_language),
    employment_type      = sqlc.arg(employment_type),
    education_level      = sqlc.arg(education_level),
    english_level        = sqlc.arg(english_level),
    experience_years_min = sqlc.arg(experience_years_min),
    role_fingerprint = sqlc.arg(role_fingerprint),
    company_slug     = sqlc.arg(company_slug),
    company_slug_folded = replace(sqlc.arg(company_slug), '-', ''),
    updated_at = CASE
        WHEN role_fingerprint IS DISTINCT FROM sqlc.arg(role_fingerprint) THEN now()
        ELSE updated_at
    END
WHERE id = sqlc.arg(id);
