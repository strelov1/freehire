-- name: ListJobs :many
-- Newest-added first: created_at is when the job entered the catalogue (stable
-- across re-ingests), so fresh ingests surface on top regardless of how old the
-- platform's posted_at is. id breaks ties within one ingest batch.
SELECT *
FROM jobs
WHERE closed_at IS NULL AND duplicate_of IS NULL
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

-- name: ListJobsBySourceAfter :many
-- Keyset scan over one provider's rows, for cmd/backfill-justjoin: pages by the immutable
-- primary key (concurrent writes can't skip or repeat rows) filtered to a single source. Returns
-- closed rows too — a one-time backfill of a missing description fills open and closed alike.
SELECT *
FROM jobs
WHERE source = sqlc.arg(source) AND id > sqlc.arg(after_id)
ORDER BY id
LIMIT sqlc.arg(batch_size);

-- name: UpdateJobDescription :execrows
-- Targeted description rewrite for cmd/backfill-justjoin: sets the description and the refreshed
-- content_hash (recomputed in Go from the row's indexed fields with the new description) so the
-- row re-indexes. Stamps updated_at so `reindex --since` also captures it. Only the description
-- and hash move; the deterministic facets are re-derived separately by cmd/backfill-derive.
UPDATE jobs
SET description  = sqlc.arg(description),
    content_hash = sqlc.arg(content_hash),
    updated_at   = now()
WHERE id = sqlc.arg(id);

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

-- name: ListJobIDsAfter :many
-- Id-only projection of ListJobsByIDAfter, used as the corruption-degrade path:
-- when a full SELECT * batch faults on a corrupted TOAST value (SQLSTATE XX001),
-- the scan re-reads the same window as bare ids (id is never toasted, so this
-- never faults) and then fetches each row individually to isolate and skip the
-- unreadable one.
SELECT id
FROM jobs
WHERE id > sqlc.arg(after_id)
ORDER BY id
LIMIT sqlc.arg(batch_size);

-- name: ListJobIDsUpdatedAfter :many
-- Id-only projection of ListJobsUpdatedAfter — the corruption-degrade path for the
-- incremental (`reindex --since`) scan, mirroring ListJobIDsAfter.
SELECT id
FROM jobs
WHERE id > sqlc.arg(after_id) AND updated_at >= sqlc.arg(since)
ORDER BY id
LIMIT sqlc.arg(batch_size);

-- name: ListOpenJobsPostedAfter :many
-- Freshness-scoped keyset scan for `reindex --semantic --posted-within`: open jobs
-- whose effective posting date (COALESCE(posted_at, created_at) — the same date
-- jobview derives and the search doc's posted_ts encodes) is at or after the cutoff.
-- The in-engine embedder cannot embed the whole open catalogue in reasonable time, so
-- the semantic index covers only this fresh window; being a swap rebuild it also drops
-- jobs that have since aged out. Open-only (closed_at IS NULL): a swap rebuild never
-- holds closed jobs, so unlike ListJobsUpdatedAfter there is nothing to delete. Served
-- by jobs_open_enrich_freshness_idx (COALESCE(posted_at, created_at) DESC WHERE open).
SELECT *
FROM jobs
WHERE id > sqlc.arg(after_id) AND closed_at IS NULL AND COALESCE(posted_at, created_at) >= sqlc.arg(posted_since)
ORDER BY id
LIMIT sqlc.arg(batch_size);

-- name: ListOpenJobIDsPostedAfter :many
-- Id-only projection of ListOpenJobsPostedAfter — the corruption-degrade path for the
-- freshness-scoped semantic scan, mirroring ListJobIDsAfter.
SELECT id
FROM jobs
WHERE id > sqlc.arg(after_id) AND closed_at IS NULL AND COALESCE(posted_at, created_at) >= sqlc.arg(posted_since)
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

-- name: GetJobBySourceExternalID :one
-- Load a job by its dedup identity (source, external_id) — the key the Job
-- aggregate's repository loads by, mirroring the CloseJobBySourceExternalID key.
SELECT *
FROM jobs
WHERE source = $1 AND external_id = $2;

-- name: GetJobIDBySlug :one
-- Slim slug->id lookup for the view/apply interaction path, which needs only the
-- internal id (the user_jobs FK) and must not drag the wide description/enrichment
-- columns over the wire on every silent view. GetJobBySlug (SELECT *) stays for the
-- public detail handler that renders the whole row.
SELECT id
FROM jobs
WHERE public_slug = $1;

-- name: EstimateOpenJobs :one
-- Fast approximate open-job total for the DB-backed /jobs list's meta.total. An
-- exact count(*) over ~millions of open rows was a per-request full scan; the
-- planner's estimate (see estimate_open_jobs(), migration 0033) is O(1) and
-- tracks the closed_at IS NULL filter. The total is approximate by design.
SELECT estimate_open_jobs()::bigint;

-- name: ListJobSitemapFreshest :many
-- The freshest open jobs for the sitemap: only the fields a URL needs, newest id
-- first. Ordering by id DESC (served by jobs_open_id_idx) reads the most recently
-- inserted rows, which sit at the physical end of the heap — a sequential, cache-warm
-- scan. Enumerating the whole 2.5M-row catalogue per request is heap-bound and far
-- too slow (and pollutes the buffer cache), so the sitemap ships the freshest slice;
-- fuller coverage needs a precomputed narrow table, not a live scan.
SELECT public_slug, updated_at
FROM jobs
WHERE closed_at IS NULL AND duplicate_of IS NULL
ORDER BY id DESC
LIMIT sqlc.arg(row_limit);

-- name: ListJobsByCompany :many
-- duplicate_of IS NULL collapses role-cluster reposts to their canonical row, matching
-- the /jobs list so a company page shows one card per role, not every repost.
SELECT *
FROM jobs
WHERE company_slug = $1 AND closed_at IS NULL AND duplicate_of IS NULL
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
    public_slug, countries, regions, cities, work_mode, skills, seniority, category,
    posting_language, employment_type, education_level, english_level, experience_years_min,
    content_hash, role_fingerprint
) VALUES (
    sqlc.arg(source), sqlc.arg(external_id), sqlc.arg(url), sqlc.arg(title),
    sqlc.arg(company), sqlc.arg(company_slug), sqlc.arg(location), sqlc.arg(remote),
    sqlc.arg(description), sqlc.arg(posted_at),
    sqlc.arg(public_slug),
    COALESCE(sqlc.arg(countries)::text[], '{}'), COALESCE(sqlc.arg(regions)::text[], '{}'), COALESCE(sqlc.arg(cities)::text[], '{}'),
    sqlc.arg(work_mode), COALESCE(sqlc.arg(skills)::text[], '{}'), sqlc.arg(seniority), sqlc.arg(category),
    sqlc.arg(posting_language), sqlc.arg(employment_type), sqlc.arg(education_level), sqlc.arg(english_level), sqlc.arg(experience_years_min),
    sqlc.arg(content_hash), sqlc.arg(role_fingerprint)
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
    -- description comes from a separate, best-effort detail fetch (some adapters,
    -- e.g. habr_career, load it from a per-vacancy page that an anti-bot layer can
    -- intermittently fail). A failed fetch yields an empty description but still
    -- upserts the job, so writing EXCLUDED unconditionally would let a transient
    -- failure wipe a good description. Keep the stored value when the incoming one is
    -- empty; a non-empty description still overwrites, so real edits propagate.
    -- (content_hash below stays the incoming fingerprint; the incremental indexer
    -- rebuilds its doc from the RETURNING row, which carries this preserved value.)
    description  = COALESCE(NULLIF(EXCLUDED.description, ''), jobs.description),
    posted_at    = EXCLUDED.posted_at,
    countries    = EXCLUDED.countries,
    regions      = EXCLUDED.regions,
    cities       = EXCLUDED.cities,
    work_mode    = EXCLUDED.work_mode,
    skills       = EXCLUDED.skills,
    seniority    = EXCLUDED.seniority,
    category     = EXCLUDED.category,
    posting_language     = EXCLUDED.posting_language,
    employment_type      = EXCLUDED.employment_type,
    education_level      = EXCLUDED.education_level,
    english_level        = EXCLUDED.english_level,
    experience_years_min = EXCLUDED.experience_years_min,
    content_hash = EXCLUDED.content_hash,
    -- role_fingerprint is the repost-identity (internal/jobhash.RoleFingerprint):
    -- refreshed on re-ingest so a title/description edit re-clusters the role.
    role_fingerprint = EXCLUDED.role_fingerprint,
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

-- name: RoleClusterCount :one
-- The job-reality repost/mass-posting counts for one role cluster: how many postings
-- of the same role (by role_fingerprint within a company) exist of any status
-- (repost_count = repost history) and how many are still open (mass_count = concurrent
-- mass-posting). A NULL/empty fingerprint is excluded so unfingerprinted rows never
-- cluster together; a lookup miss means a unique role (count 1). Used by the
-- incremental index push and the single-job detail read.
SELECT
    COUNT(*)::bigint AS repost_count,
    COUNT(*) FILTER (WHERE closed_at IS NULL)::bigint AS mass_count
FROM jobs
WHERE company_slug = sqlc.arg(company_slug)
  AND role_fingerprint = sqlc.arg(role_fingerprint)
  AND role_fingerprint <> '';

-- name: ListRoleClusterCopies :many
-- The open postings sharing a role cluster (company_slug + role_fingerprint) with the
-- anchor job — the "N openings across cities" list for a collapsed role. Each copy keeps
-- its own location and apply URL, so a seeker picks their city; the anchor itself is
-- included (it is one of the openings). Ordered by location. An empty-fingerprint anchor
-- clusters with no one and returns nothing.
SELECT j.public_slug, j.location, j.url, j.posted_at,
    COUNT(*) OVER()::bigint AS total
FROM jobs j
JOIN jobs anchor ON anchor.id = sqlc.arg(job_id)
WHERE j.company_slug = anchor.company_slug
  AND j.role_fingerprint = anchor.role_fingerprint
  AND anchor.role_fingerprint <> ''
  AND j.closed_at IS NULL
ORDER BY j.location, j.id
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: RoleClusterCountsAll :many
-- The whole-catalogue role-cluster counts in one aggregate pass, for the reindex to
-- build its (company_slug, role_fingerprint) -> counts lookup once. Only clusters with
-- more than one posting are returned (singletons are the count-1 default a lookup miss
-- already implies), keeping the map small. NULL/empty fingerprints are excluded.
SELECT
    company_slug,
    role_fingerprint,
    COUNT(*)::bigint AS repost_count,
    COUNT(*) FILTER (WHERE closed_at IS NULL)::bigint AS mass_count
FROM jobs
WHERE role_fingerprint IS NOT NULL AND role_fingerprint <> ''
GROUP BY company_slug, role_fingerprint
HAVING COUNT(*) > 1;

-- name: CompaniesWithRoleClusters :many
-- Company slugs whose role-duplicate markers may need recomputing: a company with an
-- open role cluster (>1 posting sharing a fingerprint) to collapse, OR one still
-- carrying an open marker that may need clearing (its cluster shrank). The recompute
-- processes these ONE COMPANY AT A TIME (RecomputeRoleDuplicatesForCompany) in short
-- transactions, so it never holds a table-wide lock that would stall concurrent ingest
-- crawls (a whole-table UPDATE did: it locked ~1.4M rows for minutes).
SELECT company_slug FROM jobs
WHERE closed_at IS NULL AND company_slug <> '' AND role_fingerprint IS NOT NULL AND role_fingerprint <> ''
GROUP BY company_slug, role_fingerprint
HAVING COUNT(*) > 1
UNION
SELECT DISTINCT company_slug FROM jobs
WHERE closed_at IS NULL AND company_slug <> '' AND duplicate_of IS NOT NULL;

-- name: RecomputeRoleDuplicatesForCompany :execrows
-- The per-company slice of the role-duplicate recompute. Canon = min(id) among the
-- company's open rows sharing a role_fingerprint; the canon and any singleton/empty-fp
-- row get duplicate_of NULL, the other reposts point to the canon. Rows are never
-- deleted, so the reality counts are untouched. Scoped to one company_slug so it locks
-- only that company's rows briefly; the (company_slug, role_fingerprint) index makes the
-- aggregation a range scan. The IS DISTINCT FROM guard makes re-runs cheap and
-- idempotent, and a closed canon fails over to the next min(id) on the next run.
WITH canon AS (
    SELECT jobs.role_fingerprint, MIN(jobs.id) AS canon_id, COUNT(*) AS n
    FROM jobs
    WHERE jobs.company_slug = sqlc.arg(company)
      AND jobs.closed_at IS NULL AND jobs.role_fingerprint IS NOT NULL AND jobs.role_fingerprint <> ''
    GROUP BY jobs.role_fingerprint
),
target AS (
    SELECT j.id,
        CASE WHEN c.n > 1 AND j.id <> c.canon_id THEN c.canon_id END AS new_dup
    FROM jobs j
    JOIN canon c ON j.role_fingerprint = c.role_fingerprint
    WHERE j.company_slug = sqlc.arg(company) AND j.closed_at IS NULL
)
UPDATE jobs j
SET duplicate_of = t.new_dup,
    updated_at   = now()
FROM target t
WHERE j.id = t.id
  AND j.duplicate_of IS DISTINCT FROM t.new_dup;

-- name: CompaniesWithAggregatorPostings :many
-- Company slugs with at least one OPEN aggregator posting — the drive list for the
-- cross-source aggregator suppression pass. An open aggregator row is a candidate whether
-- it still needs suppressing OR needs releasing (its ATS twin closed), so one predicate
-- covers both. Processed one company at a time (SuppressAggregatorDuplicatesForCompany),
-- mirroring the role-duplicate recompute's lock discipline.
SELECT DISTINCT company_slug FROM jobs
WHERE closed_at IS NULL AND company_slug <> ''
  AND source = ANY(sqlc.arg(aggregators)::text[]);

-- name: SuppressAggregatorDuplicatesForCompany :execrows
-- The per-company slice of the cross-source aggregator suppression. An open aggregator
-- posting is marked duplicate_of an open CANONICAL ATS (non-aggregator) posting of the
-- same company, equal normalized title, and compatible country (countries overlap, or
-- either side empty — the geography dictionary is sparse, so an unresolved side must not
-- veto). The ATS row is never touched, so it stays canonical. Candidate aggregator rows
-- are those that are canonical OR already point at a non-aggregator row (i.e. suppressed
-- by THIS pass) — an aggregator repost pointed at another aggregator by the role pass is
-- left alone. A candidate with no ATS twin resolves to NULL, so a closed twin releases
-- its aggregator copy back into search/embedding/enrichment. min(id) picks a stable
-- target; the IS DISTINCT FROM guard makes re-runs cheap and idempotent. Run AFTER
-- RecomputeRoleDuplicatesForCompany so ATS reposts have already collapsed to their canon.
WITH ats AS (
    SELECT jobs.id,
           btrim(regexp_replace(lower(jobs.title), '[^a-z0-9]+', ' ', 'g')) AS ntitle,
           jobs.countries
    FROM jobs
    WHERE jobs.company_slug = sqlc.arg(company)
      AND jobs.closed_at IS NULL AND jobs.duplicate_of IS NULL
      AND NOT (jobs.source = ANY(sqlc.arg(aggregators)::text[]))
),
agg AS (
    SELECT a.id,
           btrim(regexp_replace(lower(a.title), '[^a-z0-9]+', ' ', 'g')) AS ntitle,
           a.countries
    FROM jobs a
    WHERE a.company_slug = sqlc.arg(company)
      AND a.closed_at IS NULL
      AND a.source = ANY(sqlc.arg(aggregators)::text[])
      AND (
          a.duplicate_of IS NULL
          OR EXISTS (
              SELECT 1 FROM jobs p
              WHERE p.id = a.duplicate_of
                AND NOT (p.source = ANY(sqlc.arg(aggregators)::text[]))
          )
      )
),
target AS (
    -- LEFT JOIN + MIN (not a correlated subquery) so the match is a hash join on ntitle,
    -- O(agg + ats) instead of O(agg * ats) — a company with thousands of same-title rows
    -- (a hotel chain's ATS + its aggregator copies) would otherwise scan quadratically.
    -- No match (including an empty ntitle, excluded in the ON) yields NULL, which releases
    -- a previously-suppressed row whose ATS twin has closed.
    SELECT a.id, MIN(t.id) AS new_dup
    FROM agg a
    LEFT JOIN ats t
      ON t.ntitle = a.ntitle
     AND a.ntitle <> ''
     AND (t.countries && a.countries
          OR cardinality(t.countries) = 0
          OR cardinality(a.countries) = 0)
    GROUP BY a.id
)
UPDATE jobs j
SET duplicate_of = t.new_dup,
    updated_at   = now()
FROM target t
WHERE j.id = t.id
  AND j.duplicate_of IS DISTINCT FROM t.new_dup;

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
    public_slug, countries, regions, cities, work_mode, skills, seniority, category,
    posting_language, employment_type, education_level, english_level, experience_years_min,
    created_by
) VALUES (
    sqlc.arg(source), sqlc.arg(external_id), sqlc.arg(url), sqlc.arg(title),
    sqlc.arg(company), sqlc.arg(company_slug), sqlc.arg(location), sqlc.arg(remote),
    sqlc.arg(description), sqlc.arg(posted_at),
    sqlc.arg(public_slug),
    COALESCE(sqlc.arg(countries)::text[], '{}'), COALESCE(sqlc.arg(regions)::text[], '{}'), COALESCE(sqlc.arg(cities)::text[], '{}'),
    sqlc.arg(work_mode), COALESCE(sqlc.arg(skills)::text[], '{}'),
    sqlc.arg(seniority), sqlc.arg(category),
    sqlc.arg(posting_language), sqlc.arg(employment_type), sqlc.arg(education_level), sqlc.arg(english_level), sqlc.arg(experience_years_min),
    sqlc.arg(created_by)::bigint
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
    cities       = EXCLUDED.cities,
    work_mode    = EXCLUDED.work_mode,
    skills       = EXCLUDED.skills,
    seniority    = EXCLUDED.seniority,
    category     = EXCLUDED.category,
    posting_language     = EXCLUDED.posting_language,
    employment_type      = EXCLUDED.employment_type,
    education_level      = EXCLUDED.education_level,
    english_level        = EXCLUDED.english_level,
    experience_years_min = EXCLUDED.experience_years_min,
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
    cities       = COALESCE(sqlc.arg(cities)::text[], '{}'),
    work_mode    = sqlc.arg(work_mode),
    skills       = COALESCE(sqlc.arg(skills)::text[], '{}'),
    seniority    = sqlc.arg(seniority),
    category     = sqlc.arg(category),
    posting_language     = sqlc.arg(posting_language),
    employment_type      = sqlc.arg(employment_type),
    education_level      = sqlc.arg(education_level),
    english_level        = sqlc.arg(english_level),
    experience_years_min = sqlc.arg(experience_years_min),
    updated_by   = sqlc.arg(updated_by)::bigint,
    updated_at   = now()
WHERE public_slug = sqlc.arg(public_slug) AND created_by IS NOT NULL
RETURNING *;

-- name: CloseUnseenJobs :execrows
-- Post-ingest sweep (see job-lifecycle spec): close every open job of ONE source not
-- seen since the cutoff, scoped to the company slugs the run actually crawled. Scoped
-- by source because ingest runs per provider (a greenhouse run must not close jobs
-- another provider owns), and by company_slug because a run may crawl only a SUBSET of
-- a provider's boards — a partial or targeted run (or a full crawl of a huge provider
-- that times out and only completes some boards) must not close the companies it never
-- touched. The caller passes the crawled slugs and owns the grace window (cutoff =
-- now() - window), so neither a failed nor a partial crawl mass-closes a catalogue.
UPDATE jobs
SET closed_at  = now(),
    updated_at = now()
WHERE closed_at IS NULL
  AND source = sqlc.arg(source)
  AND last_seen_at < sqlc.arg(cutoff)
  AND company_slug = ANY(sqlc.arg(company_slugs)::text[]);

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

-- name: ExistingExternalIDs :many
-- Seen-set for a hydrating source (see source-ingest): all external_ids stored for one
-- provider, so an adapter with expensive per-posting detail (justjoin, ~20k live offers)
-- fetches detail only for postings the catalogue does not already have. Closed rows are
-- included — a closed posting is still "seen" (no need to re-fetch its detail; a reappearance
-- reopens it via the upsert regardless). Keyed by source alone; the caller namespaces the
-- adapter's raw posting id to match the stored external_id.
SELECT external_id FROM jobs WHERE source = sqlc.arg(source);

-- name: TouchJob :one
-- Liveness refresh for a hydrating source's already-ingested posting (see source-ingest): the
-- crawl re-listed the offer but fetched no fresh content (detail is fetched only for new
-- offers), so refresh last_seen_at and reopen if it had been closed — WITHOUT touching the
-- content columns. A full upsert of the content-less listing would re-derive the deterministic
-- facets from an empty description and wipe the row's hydrated description/skills. This is the
-- reopen half of UpsertJob's ON CONFLICT, minus every content write. RETURNING company_slug so
-- the caller records the company into the crawled-set that scopes the post-run unseen sweep —
-- exactly as UpsertJob's write path does — otherwise a company whose offers were all touched
-- (not newly saved) would drop out of the sweep and its removed offers would never close.
UPDATE jobs
SET last_seen_at = now(),
    closed_at    = NULL,
    -- A reopen resets the strike count so a single later expired probe can't immediately
    -- re-close it, mirroring UpsertJob.
    liveness_strikes = CASE WHEN closed_at IS NOT NULL THEN 0 ELSE liveness_strikes END,
    updated_at   = now()
WHERE source = sqlc.arg(source) AND external_id = sqlc.arg(external_id)
RETURNING company_slug;

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
-- enrichment, gated on the same conditions the backfill uses (unenriched or below the
-- target schema version, and a non-blacklisted category), so an already-enriched job
-- is not re-queued and a confidently non-technical role (exclude_categories =
-- enrich.NonTechCategories) never consumes LLM budget. category is NOT NULL DEFAULT '',
-- so an empty/unrecognized category still enqueues (empty string <> ALL). Idempotent
-- via the outbox's UNIQUE (job_id, target_version). Run in the same transaction as the
-- job's UpsertJob so a newly ingested job is queued atomically with its write.
INSERT INTO enrichment_outbox (job_id, target_version)
SELECT id, sqlc.arg(target_version)::int
FROM jobs
WHERE id = sqlc.arg(job_id)::bigint
  AND (enriched_at IS NULL OR enrichment_version < sqlc.arg(target_version)::int)
  AND category <> ALL(COALESCE(sqlc.arg(exclude_categories)::text[], '{}'))
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
-- english_level, and experience_years_min — from the row's raw content
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
    cities    = COALESCE(sqlc.arg(cities)::text[], '{}'),
    work_mode = sqlc.arg(work_mode),
    skills    = COALESCE(sqlc.arg(skills)::text[], '{}'),
    seniority = sqlc.arg(seniority),
    category  = sqlc.arg(category),
    posting_language     = sqlc.arg(posting_language),
    employment_type      = sqlc.arg(employment_type),
    education_level      = sqlc.arg(education_level),
    english_level        = sqlc.arg(english_level),
    experience_years_min = sqlc.arg(experience_years_min)
WHERE id = sqlc.arg(id);
