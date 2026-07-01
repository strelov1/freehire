-- name: ListJobs :many
-- Newest-added first: created_at is when the job entered the catalogue (stable
-- across re-ingests), so fresh ingests surface on top regardless of how old the
-- platform's posted_at is. id breaks ties within one ingest batch.
SELECT *
FROM jobs
WHERE closed_at IS NULL
ORDER BY created_at DESC, id DESC
LIMIT $1 OFFSET $2;

-- name: ListJobsByIDAfter :many
-- Keyset scan for the reindex command: pages by the immutable primary key, so
-- concurrent inserts/updates (which shift posted_at ordering) cannot make the
-- scan skip or repeat rows the way OFFSET pagination would.
SELECT *
FROM jobs
WHERE id > sqlc.arg(after_id)
ORDER BY id
LIMIT sqlc.arg(batch_size);

-- name: ListJobsUpdatedAfter :many
-- Incremental keyset scan for `reindex --since`: like ListJobsByIDAfter but only
-- rows changed at or after the cutoff. Every write path (UpsertJob, the close
-- sweeps, SetJobEnrichment, UpdateJobFacets) stamps updated_at = now(), so this
-- captures new, re-crawled, closed, and re-enriched jobs — enough to bring an
-- index current without re-pushing the whole table. Returns closed rows too, so
-- the caller deletes a freshly-closed job from the index.
SELECT *
FROM jobs
WHERE id > sqlc.arg(after_id) AND updated_at >= sqlc.arg(since)
ORDER BY id
LIMIT sqlc.arg(batch_size);

-- name: GetJob :one
SELECT *
FROM jobs
WHERE id = $1;

-- name: GetJobBySlug :one
SELECT *
FROM jobs
WHERE public_slug = $1;

-- name: GetJobIDBySlug :one
-- Slim slug->id lookup for the view/apply interaction path, which needs only the
-- internal id (the user_jobs FK) and must not drag the wide description/enrichment
-- columns over the wire on every silent view. GetJobBySlug (SELECT *) stays for the
-- public detail handler that renders the whole row.
SELECT id
FROM jobs
WHERE public_slug = $1;

-- name: CountJobs :one
SELECT count(*)
FROM jobs
WHERE closed_at IS NULL;

-- name: ListJobSitemap :many
-- Slim keyset page for the sitemap: only the fields a sitemap URL needs, open jobs
-- only, cursored by the immutable primary key so a chunk is a bounded index scan
-- (never a deep OFFSET over millions of rows).
SELECT id, public_slug, updated_at
FROM jobs
WHERE closed_at IS NULL AND id > sqlc.arg(after_id)
ORDER BY id
LIMIT sqlc.arg(batch_size);

-- name: JobSitemapBoundaries :many
-- The id ending every full chunk of `chunk_size` open jobs (ordered by id),
-- excluding the final row, so the sitemap index can list each sub-sitemap's keyset
-- cursor without the client walking the whole catalogue.
SELECT id FROM (
  SELECT id,
         row_number() OVER (ORDER BY id) AS rn,
         count(*) OVER () AS total
  FROM jobs
  WHERE closed_at IS NULL
) t
WHERE rn % sqlc.arg(chunk_size)::bigint = 0 AND rn < total
ORDER BY id;

-- name: ListJobsByCompany :many
SELECT *
FROM jobs
WHERE company_slug = $1 AND closed_at IS NULL
ORDER BY created_at DESC, id DESC
LIMIT $2 OFFSET $3;

-- name: UpsertJob :one
-- Single atomic write: upsert the company (only when the slug is non-empty,
-- via the WHERE on the SELECT) and the job together, keeping the "one write =
-- one job" property of the pipeline's write path.
-- The enrichment columns are deliberately NOT written here: ingest carries no
-- enrichment, so a new row takes the table defaults ('{}' / NULL / 0) and a
-- re-ingest leaves any existing enrichment untouched. SetJobEnrichment (the
-- enrichment worker) is the sole writer of those columns.
-- countries/regions ARE written here: they are source facts parsed from the
-- location, not enrichment. COALESCE maps a nil arg to '{}', so a location that
-- yields no geography stores empty arrays (the columns are NOT NULL).
-- content_hash is the incremental-index change signal (internal/jobhash): the
-- `existing` CTE captures the row's pre-update hash (snapshot from before this
-- statement), so RETURNING can report whether the write inserted a new row
-- (`inserted`) or changed its searchable content (`changed`, true on insert and
-- for a legacy NULL hash). A re-ingest that only bumps last_seen_at reports both
-- false and needs no re-push to the search index.
WITH existing AS (
    SELECT content_hash AS old_hash, true AS existed FROM jobs
    WHERE source = sqlc.arg(source) AND external_id = sqlc.arg(external_id)
),
company_upsert AS (
    INSERT INTO companies (slug, name)
    SELECT sqlc.arg(company_slug), sqlc.arg(company)
    WHERE sqlc.arg(company_slug) <> ''
    ON CONFLICT (slug) DO UPDATE SET
        name       = EXCLUDED.name,
        updated_at = now()
)
INSERT INTO jobs (
    source, external_id, url, title, company, company_slug, location, remote, description, posted_at,
    public_slug, countries, regions, work_mode, skills, seniority, category,
    posting_language, employment_type, education_level, experience_years_min,
    remote_unspecified, content_hash
) VALUES (
    sqlc.arg(source), sqlc.arg(external_id), sqlc.arg(url), sqlc.arg(title),
    sqlc.arg(company), sqlc.arg(company_slug), sqlc.arg(location), sqlc.arg(remote),
    sqlc.arg(description), sqlc.arg(posted_at),
    sqlc.arg(public_slug),
    COALESCE(sqlc.arg(countries)::text[], '{}'), COALESCE(sqlc.arg(regions)::text[], '{}'),
    sqlc.arg(work_mode), COALESCE(sqlc.arg(skills)::text[], '{}'), sqlc.arg(seniority), sqlc.arg(category),
    sqlc.arg(posting_language), sqlc.arg(employment_type), sqlc.arg(education_level), sqlc.arg(experience_years_min),
    sqlc.arg(remote_unspecified), sqlc.arg(content_hash)
)
-- public_slug is deliberately NOT in the DO UPDATE SET: the slug is minted once
-- at insert and is the row's stable public identity. Re-ingest of the same
-- (source, external_id) must not rewrite it, so external links stay valid even
-- if the slug builder changes later (that would be a deliberate migration).
ON CONFLICT (source, external_id) DO UPDATE SET
    url          = EXCLUDED.url,
    title        = EXCLUDED.title,
    company      = EXCLUDED.company,
    company_slug = EXCLUDED.company_slug,
    location     = EXCLUDED.location,
    remote       = EXCLUDED.remote,
    description  = EXCLUDED.description,
    posted_at    = EXCLUDED.posted_at,
    countries    = EXCLUDED.countries,
    regions      = EXCLUDED.regions,
    work_mode    = EXCLUDED.work_mode,
    skills       = EXCLUDED.skills,
    seniority    = EXCLUDED.seniority,
    category     = EXCLUDED.category,
    posting_language     = EXCLUDED.posting_language,
    employment_type      = EXCLUDED.employment_type,
    education_level      = EXCLUDED.education_level,
    experience_years_min = EXCLUDED.experience_years_min,
    remote_unspecified   = EXCLUDED.remote_unspecified,
    content_hash = EXCLUDED.content_hash,
    -- The crawl saw the posting: refresh liveness and reopen if it was closed. A
    -- reopen (the row was closed) resets the strike count so a single later expired
    -- probe can't immediately re-close it — the two-strike grace survives a reopen.
    last_seen_at = now(),
    closed_at    = NULL,
    liveness_strikes = CASE WHEN jobs.closed_at IS NOT NULL THEN 0 ELSE jobs.liveness_strikes END,
    updated_at   = now()
RETURNING sqlc.embed(jobs),
    NOT COALESCE((SELECT existed FROM existing), false) AS inserted,
    ((SELECT old_hash FROM existing) IS DISTINCT FROM sqlc.arg(content_hash)) AS changed;

-- name: PropagateCollectionsToJobs :execrows
-- Denormalize each company's curated-collection set onto its jobs, so the search
-- facet (jobs.collections) reflects current membership. Run by cmd/import-collections
-- after it writes companies.collections. updated_at is bumped so `reindex --since`
-- picks the changed rows up; the IS DISTINCT FROM guard skips unchanged rows, making
-- re-runs idempotent and cheap.
UPDATE jobs
SET collections = c.collections,
    updated_at  = now()
FROM companies c
WHERE jobs.company_slug = c.slug
  AND jobs.collections IS DISTINCT FROM c.collections;

-- name: UpsertManualJob :one
-- Moderator-authored write: the hand-curated analogue of UpsertJob. source is the
-- posting's real origin (e.g. 'workatastartup'), supplied by the moderator and
-- defaulting to 'manual'; the dedup key is (source, external_id = url), so re-POSTing
-- the same URL updates the row idempotently instead of duplicating it. The manual
-- provenance is recorded by created_by (set here, NULL for every automated source) —
-- not by the source value. created_by is stamped once at insert; updated_by is
-- (re)written on the conflict update. Like UpsertJob, public_slug is minted once and
-- never rewritten, and the enrichment columns are left to SetJobEnrichment. The conflict
-- reopens a previously closed posting (closed_at = NULL) since the moderator is
-- re-asserting it.
WITH company_upsert AS (
    INSERT INTO companies (slug, name)
    SELECT sqlc.arg(company_slug), sqlc.arg(company)
    WHERE sqlc.arg(company_slug) <> ''
    ON CONFLICT (slug) DO UPDATE SET
        name       = EXCLUDED.name,
        updated_at = now()
)
INSERT INTO jobs (
    source, external_id, url, title, company, company_slug, location, remote, description, posted_at,
    public_slug, countries, regions, work_mode, skills, seniority, category,
    posting_language, employment_type, education_level, experience_years_min,
    remote_unspecified, created_by
) VALUES (
    sqlc.arg(source), sqlc.arg(external_id), sqlc.arg(url), sqlc.arg(title),
    sqlc.arg(company), sqlc.arg(company_slug), sqlc.arg(location), sqlc.arg(remote),
    sqlc.arg(description), sqlc.arg(posted_at),
    sqlc.arg(public_slug),
    COALESCE(sqlc.arg(countries)::text[], '{}'), COALESCE(sqlc.arg(regions)::text[], '{}'),
    sqlc.arg(work_mode), COALESCE(sqlc.arg(skills)::text[], '{}'),
    sqlc.arg(seniority), sqlc.arg(category),
    sqlc.arg(posting_language), sqlc.arg(employment_type), sqlc.arg(education_level), sqlc.arg(experience_years_min),
    sqlc.arg(remote_unspecified), sqlc.arg(created_by)::bigint
)
ON CONFLICT (source, external_id) DO UPDATE SET
    url          = EXCLUDED.url,
    title        = EXCLUDED.title,
    company      = EXCLUDED.company,
    company_slug = EXCLUDED.company_slug,
    location     = EXCLUDED.location,
    remote       = EXCLUDED.remote,
    description  = EXCLUDED.description,
    posted_at    = EXCLUDED.posted_at,
    countries    = EXCLUDED.countries,
    regions      = EXCLUDED.regions,
    work_mode    = EXCLUDED.work_mode,
    skills       = EXCLUDED.skills,
    seniority    = EXCLUDED.seniority,
    category     = EXCLUDED.category,
    posting_language     = EXCLUDED.posting_language,
    employment_type      = EXCLUDED.employment_type,
    education_level      = EXCLUDED.education_level,
    experience_years_min = EXCLUDED.experience_years_min,
    remote_unspecified   = EXCLUDED.remote_unspecified,
    updated_by   = sqlc.arg(updated_by)::bigint,
    -- A moderator re-create reopens the job; reset the strike count too so the
    -- two-strike liveness grace survives a reopen (see UpsertJob).
    closed_at    = NULL,
    liveness_strikes = CASE WHEN jobs.closed_at IS NOT NULL THEN 0 ELSE jobs.liveness_strikes END,
    updated_at   = now()
RETURNING *;

-- name: UpdateManualJob :one
-- Moderator edit of a hand-curated job, addressed by public_slug and scoped to
-- created_by IS NOT NULL so this path can only rewrite a moderator-authored posting,
-- never an automated-source (ingest/telegram) one — regardless of the declared source.
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
    location     = sqlc.arg(location),
    remote       = sqlc.arg(remote),
    description  = sqlc.arg(description),
    posted_at    = sqlc.arg(posted_at),
    countries    = COALESCE(sqlc.arg(countries)::text[], '{}'),
    regions      = COALESCE(sqlc.arg(regions)::text[], '{}'),
    work_mode    = sqlc.arg(work_mode),
    skills       = COALESCE(sqlc.arg(skills)::text[], '{}'),
    seniority    = sqlc.arg(seniority),
    category     = sqlc.arg(category),
    posting_language     = sqlc.arg(posting_language),
    employment_type      = sqlc.arg(employment_type),
    education_level      = sqlc.arg(education_level),
    experience_years_min = sqlc.arg(experience_years_min),
    remote_unspecified   = sqlc.arg(remote_unspecified),
    updated_by   = sqlc.arg(updated_by)::bigint,
    updated_at   = now()
WHERE public_slug = sqlc.arg(public_slug) AND created_by IS NOT NULL
RETURNING *;

-- name: CloseUnseenJobs :execrows
-- Post-ingest sweep (see job-lifecycle spec): close every open job of ONE source not
-- seen since the cutoff. Scoped by source because ingest runs per provider — a
-- greenhouse run must not close jobs another provider owns and didn't crawl. The
-- caller owns the grace window (cutoff = now() - window) and the "run ingested
-- something" guard, so a failed crawl never mass-closes that source's catalogue.
UPDATE jobs
SET closed_at  = now(),
    updated_at = now()
WHERE closed_at IS NULL
  AND source = sqlc.arg(source)
  AND last_seen_at < sqlc.arg(cutoff);

-- name: CloseJobBySourceExternalID :execrows
-- Stream-driven close (see job-lifecycle): a self-closing feed source (e.g. jobtech)
-- learns of a removed posting from its incremental stream and closes it by identity,
-- rather than relying on the post-run unseen sweep (which it opts out of, since an
-- incremental stream re-reports only changed ads and so never refreshes last_seen_at
-- for the still-open ones). WHERE closed_at IS NULL keeps it idempotent; a later
-- upsert of the same (source, external_id) reopens it if the posting reappears.
UPDATE jobs
SET closed_at  = now(),
    updated_at = now()
WHERE closed_at IS NULL
  AND source = sqlc.arg(source)
  AND external_id = sqlc.arg(external_id);

-- name: CloseJobByID :execrows
-- Soft-close one job now (see job-lifecycle): a moderator resolving a report with
-- close_job=true. The third writer of closed_at, alongside the ingest sweep and the
-- liveness probe. WHERE closed_at IS NULL keeps it idempotent — a second close on an
-- already-closed job is a no-op, never an error, so it never fights the report's own
-- status guard. A later ingest upsert may legitimately reopen a board job (reopen-on-
-- reappear); that is the lifecycle's existing behavior, not a conflict.
UPDATE jobs
SET closed_at  = now(),
    updated_at = now()
WHERE id = sqlc.arg(id) AND closed_at IS NULL;

-- name: SelectOrphanLivenessCandidates :many
-- Orphan-job liveness (probe-orphan-job-liveness): open jobs whose source is NOT a
-- registered ATS board provider — the sources no ingest run re-crawls and the sweep
-- therefore never closes (telegram, habr_career, geekjob, …). The caller passes the
-- ATS provider set from the sources registry; <> ALL excludes them, so a new adapter
-- never silently becomes a probe target. Closed jobs are skipped (already not open).
SELECT id, source, url, public_slug, liveness_strikes
FROM jobs
WHERE closed_at IS NULL
  AND source <> ALL(sqlc.arg(ats_providers)::text[]);

-- name: MarkLivenessExpired :one
-- Record one expired probe: increment the strike counter and, in the same write,
-- close the job (closed_at) once it reaches the threshold the caller owns — the
-- two-strike grace that absorbs a transient death signal. Returns the new strike
-- count and closed_at so the worker can log the outcome.
UPDATE jobs
SET liveness_strikes = liveness_strikes + 1,
    closed_at = CASE
        WHEN liveness_strikes + 1 >= sqlc.arg(threshold) THEN now()
        ELSE closed_at
    END,
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING id, liveness_strikes, closed_at;

-- name: ResetLivenessStrikes :exec
-- A healthy (not-expired) probe clears any accumulated strikes, so only CONSECUTIVE
-- expired probes can close a job. Guarded to the non-zero case so probing an
-- already-clean job does not churn the row.
UPDATE jobs
SET liveness_strikes = 0
WHERE id = sqlc.arg(id) AND liveness_strikes <> 0;

-- name: UpdateJobSlugs :exec
-- One-off backfill for a deliberate slug-builder change (see the UpsertJob note on
-- why slugs are otherwise immutable). public_slug/company_slug are deterministic
-- from the row's immutable fields, so recomputing and rewriting them is idempotent.
UPDATE jobs
SET public_slug  = sqlc.arg(public_slug),
    company_slug = sqlc.arg(company_slug)
WHERE id = sqlc.arg(id);

-- name: EnqueueJobEnrichment :execrows
-- Transactional-outbox enqueue for the ingest write path: queue this one job for
-- enrichment, gated on the same condition the backfill uses (unenriched or below the
-- target schema version), so an already-enriched job is not re-queued. Idempotent via
-- the outbox's UNIQUE (job_id, target_version). Run in the same transaction as the
-- job's UpsertJob so a newly ingested job is queued atomically with its write.
INSERT INTO enrichment_outbox (job_id, target_version)
SELECT id, sqlc.arg(target_version)::int
FROM jobs
WHERE id = sqlc.arg(job_id)::bigint
  AND (enriched_at IS NULL OR enrichment_version < sqlc.arg(target_version)::int)
ON CONFLICT (job_id, target_version) DO NOTHING;

-- name: SetJobEnrichment :exec
-- Targeted enrichment write used by the enrichment command: set only the payload
-- and the provenance stamp, touching no raw source field. Kept separate from
-- UpsertJob (the ingest full-upsert path) so ingest and enrichment stay decoupled.
UPDATE jobs
SET enrichment         = sqlc.arg(enrichment),
    enriched_at        = sqlc.arg(enriched_at),
    enrichment_version = sqlc.arg(enrichment_version),
    updated_at         = now()
WHERE id = sqlc.arg(id);

-- name: UpdateJobFacets :exec
-- One-off backfill (cmd/backfill-derive): rewrite every deterministic dictionary
-- facet column — countries, regions, work_mode, skills, seniority, category, plus the
-- synthetic enrichment facets posting_language, employment_type, education_level,
-- experience_years_min, and remote_unspecified — from the row's raw content
-- (title/location/description) in one
-- pass, replacing the
-- three separate per-facet backfill writes. The facets are a pure function of the
-- raw fields, so this is idempotent. updated_at is deliberately left untouched
-- (like UpdateJobSlugs) so a backfill does not churn every row's timestamp. COALESCE
-- maps a nil array arg to '{}' for the NOT NULL array columns. work_mode is written
-- as given by the caller, which preserves an already-set (possibly
-- adapter-structured) value.
UPDATE jobs
SET countries = COALESCE(sqlc.arg(countries)::text[], '{}'),
    regions   = COALESCE(sqlc.arg(regions)::text[], '{}'),
    work_mode = sqlc.arg(work_mode),
    skills    = COALESCE(sqlc.arg(skills)::text[], '{}'),
    seniority = sqlc.arg(seniority),
    category  = sqlc.arg(category),
    posting_language     = sqlc.arg(posting_language),
    employment_type      = sqlc.arg(employment_type),
    education_level      = sqlc.arg(education_level),
    experience_years_min = sqlc.arg(experience_years_min),
    remote_unspecified   = sqlc.arg(remote_unspecified)
WHERE id = sqlc.arg(id);
