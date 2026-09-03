-- name: ListJobs :many
-- Newest-added first: created_at is when the job entered the catalogue (stable
-- across re-ingests), so fresh ingests surface on top regardless of how old the
-- platform's posted_at is. id breaks ties within one ingest batch.
--
-- AND NOT is_private excludes the jd-tailor-intake private-job path (visible only to
-- its creator, through GetJobBySlug, not this listing). The partial index backing this
-- query's ORDER BY (jobs_open_created_idx) predicates on closed_at IS NULL only, not
-- is_private — Postgres can still use it here since that predicate is implied by this
-- WHERE clause, applying the is_private filter on the (small) scanned window rather than
-- the whole table, so this stays index-served rather than degrading to a full scan.
SELECT *
FROM jobs
WHERE closed_at IS NULL AND duplicate_of IS NULL AND NOT is_private
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
-- Keyset scan over one provider's rows, for the per-source repair workers (e.g.
-- cmd/backfill-echojobs, cmd/backfill-descriptions): pages by the immutable primary key
-- (concurrent writes can't skip or repeat rows) filtered to a single source. Returns
-- closed rows too — a one-time backfill of a missing description fills open and closed alike.
SELECT *
FROM jobs
WHERE source = sqlc.arg(source) AND id > sqlc.arg(after_id)
ORDER BY id
LIMIT sqlc.arg(batch_size);

-- name: UpdateJobDescription :execrows
-- Targeted description rewrite shared by the per-source description-repair workers (e.g.
-- cmd/backfill-echojobs, cmd/backfill-descriptions): sets the description and the refreshed
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
-- sweeps, SetJobEnrichment, UpdateJobDerived on a fingerprint move) stamps
-- updated_at = now(), so this
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

-- name: FindOpenJobByURL :one
-- Resolve a job page URL to the posting stored under it — the second tier of
-- /api/v1/jobs/find, used when no (source, external_id) identity can be read out of the
-- URL. Both sides go through normalize_job_url (migration 0042), so a link differing only
-- by scheme, www., tracking query, fragment, case or trailing slash still matches; the
-- same expression backs jobs_normalized_url_idx, so this is an index lookup.
-- Scoped to open rows: a closed posting is not one to show a match card for.
--
-- Duplicates ARE matched, and answer with the posting they duplicate. One in five open
-- postings is a duplicate of another, and the candidate standing on one is looking at a
-- vacancy the catalogue knows — answering "we do not have this" because the dedup passes
-- preferred a twin is wrong from where they are sitting. This is the opposite of what the
-- search index wants (one row per group), which is why the scoping lives here and not in
-- the dedup itself.
--
-- A duplicate whose canonical row has since closed falls back to its own slug: the group's
-- chosen representative is gone, and this one is still open.
--
-- Two open rows may share a URL (an aggregator and an ATS row the dedup passes have not
-- collapsed), so a canonical row wins over a duplicate, then the most recently confirmed,
-- with id as the deterministic tiebreak.
SELECT COALESCE(canonical.public_slug, j.public_slug)
FROM jobs j
LEFT JOIN jobs canonical ON canonical.id = j.duplicate_of AND canonical.closed_at IS NULL
WHERE normalize_job_url(j.url) = normalize_job_url(@url)
  AND j.closed_at IS NULL
ORDER BY (j.duplicate_of IS NULL) DESC, j.last_seen_at DESC, j.id DESC
LIMIT 1;

-- name: GetJobIDBySlug :one
-- Slim slug->id lookup for the view/apply interaction path, which needs only the
-- internal id (the user_jobs FK) and must not drag the wide description/enrichment
-- columns over the wire on every silent view. GetJobBySlug (SELECT *) stays for the
-- public detail handler that renders the whole row.
SELECT id
FROM jobs
WHERE public_slug = $1;

-- name: GetJobDescriptionsByIDs :many
-- Batch-load full descriptions by internal id when the agent search endpoint
-- explicitly asks to include the full text instead of the truncated search
-- preview. Narrow projection (no SELECT *) so it drags only the one field it
-- patches, not the whole wide row, for a page of at most maxLimit ids.
SELECT id, description
FROM jobs
WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: GetSimilarJobIDs :one
-- Narrow read for GET /jobs/:slug/similar (internal/api/handler/similar.go): only the
-- precomputed neighbour-id list (jobs.similar_job_ids, populated by
-- cmd/similar-backfill — see semantic.sql's job_semantic_chunks section), not the
-- whole wide job row. Mirrors GetJobDescriptionsByIDs's "narrow projection for a
-- hot read path" precedent above. The list is nearest-first and, as of writing,
-- capped at cmd/similar-backfill's -similar flag (default 20, matching the API's
-- maxSimilarLimit) — the handler still re-filters it to open jobs at read time,
-- since a neighbour can close after it was computed. A job with no precomputed
-- list yet (never backfilled) comes back with a NULL/nil similar_job_ids, not an
-- error — the handler treats "not backfilled yet" and "computed empty" the same.
SELECT similar_job_ids
FROM jobs
WHERE id = sqlc.arg(id)::bigint;

-- name: LatestOpenJobAddedAt :one
-- created_at of the most recently added open, public job — the "is the pipeline
-- still writing rows" signal for the public /status endpoint. Same predicate and
-- ordering as ListJobs above, so it is served by the same jobs_open_created_idx
-- (an index scan for one row, not the full scan CountCatalogueScale needs) and
-- safe on a live request path. Wrapped in a scalar subquery so this always
-- returns exactly one row — NULL for an empty catalogue — rather than the
-- no-rows error a bare LIMIT 1 SELECT would give sqlc's :one on an empty table.
SELECT (
    SELECT created_at
    FROM jobs
    WHERE closed_at IS NULL AND duplicate_of IS NULL AND NOT is_private
    ORDER BY created_at DESC, id DESC
    LIMIT 1
) AS last_job_added_at;

-- name: CountCatalogueScale :one
-- Exact open-job and company totals for the published catalogue-scale snapshot
-- (internal/ingest/catalogstats). Deliberately the opposite trade to EstimateOpenJobs below:
-- this is a full scan and belongs only in the scheduled rollup worker, never on a
-- request path.
--
-- Both figures come from ONE statement so they describe the same instant. Counting them
-- separately would let an ingest land between the two reads and publish a company count
-- for a catalogue the job count beside it no longer describes.
--
-- The predicate is the one the public listings apply, so the totals describe exactly the
-- set a visitor can page through.
SELECT
    COUNT(*)::bigint AS open_jobs,
    COUNT(DISTINCT company_slug)::bigint AS companies
FROM jobs
WHERE closed_at IS NULL AND duplicate_of IS NULL AND NOT is_private;

-- name: EstimateOpenJobs :one
-- Fast approximate open-job total for the DB-backed /jobs list's meta.total. An
-- exact count(*) over ~millions of open rows was a per-request full scan; the
-- planner's estimate (see estimate_open_jobs(), migrations/0001_init.sql) is O(1) and
-- tracks the closed_at IS NULL filter. The total is approximate by design.
SELECT estimate_open_jobs()::bigint;

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
-- content_hash is the incremental-index change signal (internal/job/jobhash): the
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
    -- The WHERE is what keeps a crawl from rewriting one company row once per posting. Without
    -- it a board of 5,000 vacancies updated its company 5,000 times per pass: measured on prod
    -- 2026-08-02, 286M updates against 305k rows, leaving that table 32% dead tuples at 26 KB
    -- per live row. The guard sits on the UPDATE branch only, so a new company is still created.
    -- companies.updated_at is also served as a hiring company's sitemap <lastmod>, so this stops
    -- it claiming every company changed on every crawl.
    --
    -- Only this copy is guarded. The same CTE in UpsertManualJob and UpdateManualJob fires once
    -- per moderator action, where the write costs nothing worth guarding.
    ON CONFLICT (slug) DO UPDATE SET
        name       = EXCLUDED.name,
        updated_at = now()
    WHERE companies.name IS DISTINCT FROM EXCLUDED.name
)
INSERT INTO jobs (
    source, external_id, url, title, company, company_slug, company_slug_folded, location, remote, description, posted_at,
    public_slug, countries, regions, cities, work_mode, skills, seniority, category, is_tech, requires_clearance,
    posting_language, employment_type, education_level, english_level, experience_years_min,
    salary_min_source, salary_max_source, salary_currency_source, salary_period_source,
    content_hash, role_fingerprint
) VALUES (
    sqlc.arg(source), sqlc.arg(external_id), sqlc.arg(url), sqlc.arg(title),
    sqlc.arg(company), sqlc.arg(company_slug), replace(sqlc.arg(company_slug), '-', ''), sqlc.arg(location), sqlc.arg(remote),
    sqlc.arg(description), sqlc.arg(posted_at),
    sqlc.arg(public_slug),
    COALESCE(sqlc.arg(countries)::text[], '{}'), COALESCE(sqlc.arg(regions)::text[], '{}'), COALESCE(sqlc.arg(cities)::text[], '{}'),
    sqlc.arg(work_mode), COALESCE(sqlc.arg(skills)::text[], '{}'), sqlc.arg(seniority), sqlc.arg(category), sqlc.arg(is_tech), sqlc.arg(requires_clearance),
    sqlc.arg(posting_language), sqlc.arg(employment_type), sqlc.arg(education_level), sqlc.arg(english_level), sqlc.arg(experience_years_min),
    sqlc.arg(salary_min_source), sqlc.arg(salary_max_source), sqlc.arg(salary_currency_source), sqlc.arg(salary_period_source),
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
    company_slug_folded = EXCLUDED.company_slug_folded,
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
    is_tech      = EXCLUDED.is_tech,
    requires_clearance = EXCLUDED.requires_clearance,
    posting_language     = EXCLUDED.posting_language,
    employment_type      = EXCLUDED.employment_type,
    education_level      = EXCLUDED.education_level,
    english_level        = EXCLUDED.english_level,
    experience_years_min = EXCLUDED.experience_years_min,
    salary_min_source      = EXCLUDED.salary_min_source,
    salary_max_source      = EXCLUDED.salary_max_source,
    salary_currency_source = EXCLUDED.salary_currency_source,
    salary_period_source   = EXCLUDED.salary_period_source,
    content_hash = EXCLUDED.content_hash,
    -- role_fingerprint is the repost-identity (internal/job/jobhash.RoleFingerprint):
    -- refreshed on re-ingest so a title/description edit re-clusters the role.
    role_fingerprint = EXCLUDED.role_fingerprint,
    -- The crawl saw the posting: refresh liveness and reopen if it was closed. A
    -- reopen (the row was closed) resets the strike count so a single later expired
    -- probe can't immediately re-close it — the two-strike grace survives a reopen.
    last_seen_at = now(),
    closed_at    = NULL,
    closed_reason = '',
    liveness_strikes = CASE WHEN jobs.closed_at IS NOT NULL THEN 0 ELSE jobs.liveness_strikes END,
    -- A company correction (rare, but real — e.g. a slug fix on re-ingest) invalidates
    -- this job's already-precomputed similar-jobs list: it may have been computed
    -- excluding the OLD company and so now wrongly includes a same-company match.
    -- Force cmd/similar-backfill to redo it. Unconditional company_slug writes far
    -- outnumber actual changes (every re-crawl runs this branch), so this must be
    -- conditional or every re-ingest would invalidate the list for nothing.
    similar_computed_at = CASE WHEN jobs.company_slug IS DISTINCT FROM EXCLUDED.company_slug
                               THEN NULL ELSE jobs.similar_computed_at END,
    updated_at   = now()
RETURNING sqlc.embed(jobs),
    NOT COALESCE((SELECT existed FROM existing), false) AS inserted,
    ((SELECT old_hash FROM existing) IS DISTINCT FROM sqlc.arg(content_hash)) AS changed;

-- name: BackfillBoardCompany :one
-- Fills company/company_slug/company_slug_folded for rows still blank under one board of a
-- provider whose adapter sets Company statically per board (see cmd/backfill-blank-company for
-- which providers qualify and why). board_pattern is externalid.BoardPattern(board) — the
-- board's external_id namespace, so only this board's rows move. Also enqueues every touched
-- OPEN job into search_outbox, mirroring UpsertJob/EnqueueSearchOutbox's denormalized
-- job_posted_at, since this write bypasses the normal ingest path that would do it inline.
WITH updated AS (
    UPDATE jobs
    SET company             = sqlc.arg(company),
        company_slug        = sqlc.arg(company_slug),
        company_slug_folded = replace(sqlc.arg(company_slug), '-', ''),
        updated_at          = now()
    WHERE source = sqlc.arg(source)
      AND company = ''
      AND external_id LIKE sqlc.arg(board_pattern)
    RETURNING id, closed_at, COALESCE(posted_at, created_at) AS eff_posted_at
),
enqueued AS (
    INSERT INTO search_outbox (job_id, job_posted_at)
    SELECT id, eff_posted_at FROM updated WHERE closed_at IS NULL
    ON CONFLICT (job_id) DO NOTHING
    RETURNING job_id
)
SELECT (SELECT count(*) FROM updated)::bigint AS updated_count,
       (SELECT count(*) FROM enqueued)::bigint AS enqueued_count;

-- name: CountBlankCompanyByBoard :one
-- Read-only companion to BackfillBoardCompany, for --dry-run: how many rows a board's backfill
-- would touch without writing anything.
SELECT count(*)::bigint FROM jobs
WHERE source = sqlc.arg(source) AND company = '' AND external_id LIKE sqlc.arg(board_pattern);

-- name: RefreshUnchangedJob :one
-- The cheap half of the ingest write path, tried before UpsertJob: a crawl that re-sees a
-- posting identical to the stored row refreshes its liveness and writes NOTHING else. Matching
-- nothing (pgx.ErrNoRows) is the signal to run the full upsert — which is also what a brand-new
-- posting gets, since it too matches no row, and both want the same statement.
--
-- Why this exists: UpsertJob's DO UPDATE carries no WHERE, so a re-ingest rewrites the whole
-- tuple — re-TOASTing a ~2.5KB description and touching every index on the table — to move a
-- timestamp.
--
-- last_seen_at is the ONLY column written, and it is deliberately in no index, so the update is
-- heap-only and maintains none of them.
--
-- updated_at is deliberately NOT stamped, so the column comes to mean "content last changed"
-- rather than "last crawled". Two live readers see that: the jobs sitemap serves it as <lastmod>
-- (ListJobSitemapFreshest -> internal/api/handler/sitemap.go), where a timestamp that stopped
-- claiming every posting changed on every crawl is the honest signal rather than the one search
-- engines learn to discount; and jobview puts it on the public wire. It also makes
-- ListJobsUpdatedAfter viable for the first time — that query is currently dormant (no caller,
-- and cmd/reindex has no --since flag despite what its comment says), because a column stamped
-- on every crawl selects the whole catalogue and answers nothing.
--
-- The match key is (content_hash, cities, salary_*_source, english_level), not the hash alone.
-- Those are what the upsert writes that jobhash.Of does not read — a caller's structured city
-- list overrides the location-derived one, a structured salary (Lever/Ashby/Recruitee) is a
-- base fact rather than something Of hashes, and english_level gained a structured tier of its
-- own (profession.hu states it as a picklist) so it too can move while every hashed field
-- stands still. Folding them into the hash instead would change every stored content_hash at
-- once and make the first crawl after deploy rewrite and re-index the whole catalogue — 6.6M
-- rows through a queue that drains at ~9 documents a second. Note the asymmetry with
-- education_level, which IS hashed: it was hashed before either had a structured tier, and
-- moving it out now would cost exactly the storm this key exists to avoid. IS NOT DISTINCT FROM (not =) on the nullable salary bounds so two sourceless jobs
-- (both NULL) still match — a plain = would push every non-salary-bearing source off the cheap
-- path forever. Whether the key still covers every written column is enforced by
-- TestUpsertParams_CheapWriteMatchKeyCoversEveryColumnItWrites (internal/job); add a derived
-- column outside the hash and it fails there.
--
-- A NULL stored content_hash (a legacy row predating the column) compares unequal and so takes
-- the full path, which is right: nothing is known about what it holds.
--
-- closed_at IS NULL is correctness, not economy. A closed posting that reappears with identical
-- content must reach UpsertJob, which is what clears closed_at and resets the strike count.
-- Refreshing its liveness here would leave it closed while the unseen sweep kept seeing it.
--
-- RETURNING is four narrow columns, NOT sqlc.embed(jobs), and that is the point rather than an
-- economy: embedding the row would detoast the ~2.5KB description and ship semantic_embedding
-- back for every re-seen posting, adding read amplification to the path built to remove write
-- amplification. These four are everything the caller reads on this branch — the id for the
-- enrichment enqueue and the apply-form capture queue, source and company_slug for the
-- crawled-set that scopes the post-run sweep, and duplicate_of for the index-push gate. The
-- fuller row is only ever needed to BUILD a search document, which by construction this branch
-- never does. TouchJob, the hydrating-source sibling, returns company_slug alone for the same
-- reason.
UPDATE jobs
SET last_seen_at = now()
WHERE source = sqlc.arg(source)
  AND external_id = sqlc.arg(external_id)
  AND content_hash = sqlc.arg(content_hash)
  AND cities = COALESCE(sqlc.arg(cities)::text[], '{}')
  AND salary_min_source IS NOT DISTINCT FROM sqlc.arg(salary_min_source)
  AND salary_max_source IS NOT DISTINCT FROM sqlc.arg(salary_max_source)
  AND salary_currency_source = sqlc.arg(salary_currency_source)
  AND salary_period_source = sqlc.arg(salary_period_source)
  AND english_level = sqlc.arg(english_level)
  AND closed_at IS NULL
RETURNING id, source, company_slug, duplicate_of;

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

-- name: CompanyHasOtherJobs :one
-- Whether the catalog carries this company beyond the one posting named by (source,
-- external_id) — asked right after an import, to answer "is this company new to us?".
-- The board-level check (BoardTracked) cannot answer it: a company reached through an
-- ATS we do not recognise, or through a second ATS, is still a company we already carry.
-- The posting itself is excluded by its dedup identity because it is written before this
-- runs. Callers skip the question for an empty company_slug, which names nobody.
SELECT EXISTS (
    SELECT 1
    FROM jobs
    WHERE company_slug = sqlc.arg(company_slug)
      AND NOT (source = sqlc.arg(source) AND external_id = sqlc.arg(external_id))
) AS carried;

-- name: CompaniesWithFreshNonAggregatorCoverage :many
-- The ingest-time aggregator coverage gate: of the companies asked about, which still have an
-- OPEN posting from a non-aggregator source that was seen recently. A yes makes ingest DISCARD
-- the aggregator's posting unsaved, so the question has to be about the present tense — "do we
-- still crawl this employer" — not "did we ever". Without the last_seen_at cutoff a single
-- forgotten row holds its slug forever: a 2013 trakstar posting from a board that had left
-- sources/ suppressed every live Himalayas posting for a different employer of the same name
-- (issue #2315), and 22,022 slugs were held that way on prod 2026-09-02.
--
-- Companies arrive ALREADY FOLDED and are compared against the stored jobs.company_slug_folded
-- column, which UpsertJob writes beside company_slug. No exact-slug clause sits beside it
-- because none is needed: the fold is replace(company_slug,'-',''), so an exact match implies
-- a folded one. That is also what keeps the predicate on an index — see the note in
-- RecomputeAggregatorDuplicates about what the expression form costs the planner.
--
-- company_slug <> '' is not a filter on meaning (an empty slug names nobody) but the partial
-- index's own predicate, without which jobs_open_company_slug_folded_col_idx cannot be used.
--
-- NOT is_private is a correctness clause, not an inherited one. A private posting is the
-- jd-tailor-intake path — a job description a single user pasted in, visible only to them and
-- never crawled from anywhere. It can never be evidence that the catalogue still crawls an
-- employer, and counting it would let one user's pasted JD for "Acme" silently discard every
-- aggregator posting for every other Acme. The previous search-backed lookup excluded these by
-- accident (cmd/reindex drops is_private rows from the index entirely); reading the table
-- directly makes the exclusion something this query has to state.
--
-- The index's OTHER exclusions are deliberately NOT mirrored here. A non-aggregator row marked
-- duplicate_of is a repost of another posting of the same employer, and an uncategorised or
-- body-less one is still a posting we crawled from that employer's own board — all three are
-- real coverage. They are absent from the index because they are not worth SEARCHING, which is
-- a different question from whether we still crawl the company.
--
-- last_seen_at carries NO index, and must not gain one: RefreshUnchangedJob writes that column
-- alone on the hot re-crawl path precisely because it is unindexed, which keeps the update
-- heap-only. It is a recheck here, over the few open rows the folded index already selected,
-- and EXISTS stops at the first fresh one so a large employer costs no more than a small one.
SELECT asked.folded::text AS company_slug_folded
FROM unnest(sqlc.arg(folded_companies)::text[]) AS asked(folded)
WHERE EXISTS (
    SELECT 1
    FROM jobs
    WHERE jobs.company_slug_folded = asked.folded
      AND jobs.closed_at IS NULL
      AND jobs.company_slug <> ''
      AND NOT jobs.is_private
      AND jobs.last_seen_at > sqlc.arg(seen_after)
      AND NOT (jobs.source = ANY(sqlc.arg(aggregators)::text[]))
);

-- name: CanonicalJobForRole :one
-- The open canonical posting of one role cluster, asked by the import BEFORE it writes a
-- posting under the URL-keyed generic identity: a careers storefront on a company's own domain
-- fronts an ATS board we crawl, and without this the same vacancy lands twice — once from the
-- crawl, once from the pasted link. Mirrors the canon RecomputeRoleDuplicatesForCompany picks
-- (MIN(id) among the cluster's open rows) so this answer and the one a later reindex computes
-- agree rather than fight.
-- A canon must be open AND not itself a duplicate, or marking would build a chain (A -> B -> C)
-- that no reader expects. The row being written is excluded by its own dedup identity, because
-- a re-import of the same URL would otherwise find itself. Served by the partial
-- jobs_open_role_cluster_idx (migration 0013), with jobs_company_role_fingerprint_idx as the
-- non-partial fallback.
SELECT id, public_slug
FROM jobs
WHERE company_slug = sqlc.arg(company_slug)
  AND role_fingerprint = sqlc.arg(role_fingerprint)
  AND closed_at IS NULL
  AND duplicate_of IS NULL
  AND NOT (source = sqlc.arg(source) AND external_id = sqlc.arg(external_id))
ORDER BY id
LIMIT 1;

-- name: MarkJobDuplicateOfRole :one
-- Point one row at its ROLE canon. The import path only: the batch passes recompute whole
-- companies (RecomputeRoleDuplicatesForCompanies, SuppressAggregatorDuplicatesForCompanies) and
-- must keep doing so — this marks the single row an import just wrote.
--
-- Writes duplicate_of_role, not duplicate_of, and the name says so. Both callers
-- (cmd/ingest/store.go, internal/ingest/linkimport) resolve their canon through
-- jobdedup.CanonicalForRole, so this is the role verdict arriving early — the same clustering
-- the batch pass would reach hours later. A write to duplicate_of itself would not survive:
-- the derivation in migration 0115 recomputes it from the owned columns.
WITH before AS (
    SELECT b.id, b.duplicate_of AS was_duplicate_of FROM jobs b WHERE b.id = sqlc.arg(id)
),
updated AS (
    UPDATE jobs j
    SET duplicate_of_role = sqlc.arg(duplicate_of_role),
        updated_at        = now()
    WHERE j.id = sqlc.arg(id)
    RETURNING j.id,
              j.duplicate_of AS now_duplicate_of,
              COALESCE(j.posted_at, j.created_at) AS eff_posted_at
),
flipped AS (
    SELECT u.id, b.was_duplicate_of, u.now_duplicate_of, u.eff_posted_at
    FROM updated u JOIN before b ON b.id = u.id
),
dequeued AS (
    INSERT INTO search_delete_outbox (job_id)
    SELECT id FROM flipped WHERE was_duplicate_of IS NULL AND now_duplicate_of IS NOT NULL
    ON CONFLICT (job_id) DO NOTHING
),
-- Both callers pass a canon, so today this branch never fires: the import path only ever SETS
-- the role marker. It is here because the argument is nullable and the other three writers are
-- symmetric — a future caller clearing the marker through this query would otherwise leave a
-- now-canonical posting out of search until the next rebuild, silently.
requeued AS (
    INSERT INTO search_outbox (job_id, job_posted_at)
    SELECT id, eff_posted_at FROM flipped WHERE was_duplicate_of IS NOT NULL AND now_duplicate_of IS NULL
    ON CONFLICT (job_id) DO NOTHING
)
SELECT count(*)::bigint FROM updated;

-- name: ListJobCopies :many
-- The open postings the anchor's OWNER represents — the "N openings across cities" list for a
-- collapsed role. Each copy keeps its own location and apply URL, so a seeker picks their city;
-- the owner itself is included (it is one of the openings). Ordered by location.
--
-- Membership is the DUPLICATE CLOSURE, not a shared role_fingerprint, and it is deliberately
-- the same closure DuplicateClosureGeoAll unions geography over. The two must not disagree: a
-- posting whose city the canon claims in search but whose row this list omits is a location a
-- candidate can filter to and then not reach. Grouping by fingerprint could only ever see the
-- exact role pass's clusters, so a fuzzy-suppressed per-city variant — the very thing issue
-- #2225 reported — was never listed.
--
-- The anchor MAY itself be suppressed: a hidden posting stays readable by slug, which is how
-- #2225 was reported, so the walk resolves the anchor UP to its ultimate owner first and lists
-- that owner's closure. Answering with the anchor's own subtree would hand back the fragment
-- its marker happens to point at.
--
-- Cycle safety is NOT structural here, unlike the closure geography queries: those seed from
-- rows that are nobody's duplicate, which makes a cycle unreachable, but this one is handed an
-- arbitrary id. The depth bound on the upward walk is therefore load-bearing — an anchor inside
-- a marker cycle simply resolves to no owner and lists nothing.
--
-- The upward walk does not test closed_at. An anchor pointing at a closed parent still resolves
-- through it to the open owner, so the closed row costs the group nothing; the final filter is
-- what keeps closed rows out of the OUTPUT. That makes this list broader than search in one
-- direction only — it can show an open posting search does not — which is the safe direction:
-- listing a posting a candidate can apply to is never a leak, and hiding one is the complaint.
--
-- AND NOT j.is_private excludes the jd-tailor-intake private-job path: without it, a private
-- job inside the same closure would surface (slug, location, url) to anyone browsing that
-- PUBLIC job's copies — a listing leak, not merely "you'd need the direct link", which is what
-- never indexing/listing it is for.
WITH RECURSIVE up AS (
    SELECT a.id, a.duplicate_of, 0 AS depth
    FROM jobs a
    WHERE a.id = sqlc.arg(job_id)
    UNION ALL
    SELECT p.id, p.duplicate_of, u.depth + 1
    FROM up u
    JOIN jobs p ON p.id = u.duplicate_of
    WHERE u.depth < 8
),
owner AS (
    SELECT id FROM up WHERE duplicate_of IS NULL LIMIT 1
),
member AS (
    SELECT o.id AS member_id, 0 AS depth FROM owner o
    UNION ALL
    SELECT c.id, m.depth + 1
    FROM member m
    JOIN jobs c ON c.duplicate_of = m.member_id
    WHERE c.closed_at IS NULL AND m.depth < 8
)
SELECT j.public_slug, j.location, j.url, j.posted_at,
    COUNT(*) OVER()::bigint AS total
FROM member m
JOIN jobs j ON j.id = m.member_id
WHERE j.closed_at IS NULL
  AND NOT j.is_private
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

-- name: DuplicateClosureGeoAll :many
-- The whole-catalogue geography union in one pass, keyed by the id of the row that OWNS each
-- union — the searchable canonical row, widened with the geography of every OPEN row it
-- represents. What a row represents is its DUPLICATE CLOSURE: the rows whose duplicate_of
-- chain terminates at it, at any depth.
--
-- The closure, not a shared role_fingerprint, is the membership rule, because a fingerprint
-- cannot express what two of the three dedup passes do. The exact role pass clusters BY
-- fingerprint, so its members are inside their canon's closure and its behaviour is unchanged;
-- but the fuzzy-description and aggregator passes only ever act on rows the exact pass left
-- unclustered, whose fingerprints therefore DIFFER from their canon's by construction. Keyed
-- by fingerprint, their members' cities left the index with them — issue #2225, where a
-- posting open in Chestermere was readable by slug and absent from every search.
--
-- A chain of OPEN rows (a role canon that a later pass suppressed) resolves to its ultimate
-- owner, so no member's geography is stranded on an open intermediate row.
--
-- A CLOSED intermediate cuts the walk, and that is deliberate: the traversal follows open rows
-- only, so an open row behind a closed parent contributes to nobody. Such a row is invisible
-- either way — it carries a marker, so it is out of the index, and so is the closed row it
-- points at. Re-pointing it is the marker refresh's job, not this read's: the role recompute
-- picks min(id) among a cluster's OPEN rows and the fuzzy pass releases a marker whose canon
-- closed. Measured on prod 2026-09-01, 42 633 open duplicates sat behind a closed owner —
-- which is the never-released fuzzy marker this change also fixes, not a gap here.
--
-- Cycle safety is structural, not a guard. Each row has at most ONE duplicate_of, so the
-- "points at" graph has out-degree <= 1; a cycle in such a graph consists entirely of rows
-- with duplicate_of set, and every edge INTO a cycle member comes from another cycle member.
-- Seeding only from rows that are nobody's duplicate (duplicate_of IS NULL) therefore makes a
-- cycle unreachable rather than merely survivable — which is why BOTH closure queries seed
-- that way. The depth bound is a backstop for a future caller that seeds differently; today's
-- chains are at most role -> fuzzy on top of aggregator, three hops.
--
-- Only owners that represent at least one other open row are returned: a row representing
-- nobody unions to its own geography, which MergeClusterGeography already treats as a no-op,
-- and leaving it out is what keeps the rebuild's lookup map to the rows that need it. The
-- LATERAL tags each member's countries/regions/cities into one unnested stream (no cartesian
-- across the three arrays, no repeat self-join), and blanks are dropped by the FILTER.
WITH RECURSIVE member AS (
    SELECT o.id AS owner_id, o.id AS member_id, 0 AS depth
    FROM jobs o
    WHERE o.closed_at IS NULL
      AND o.duplicate_of IS NULL
      AND EXISTS (SELECT 1 FROM jobs c WHERE c.duplicate_of = o.id AND c.closed_at IS NULL)
    UNION ALL
    SELECT m.owner_id, c.id, m.depth + 1
    FROM member m
    JOIN jobs c ON c.duplicate_of = m.member_id
    WHERE c.closed_at IS NULL AND m.depth < 8
)
SELECT
    m.owner_id,
    array_agg(DISTINCT t.val) FILTER (WHERE t.kind = 'c' AND t.val <> '')::text[] AS countries,
    array_agg(DISTINCT t.val) FILTER (WHERE t.kind = 'r' AND t.val <> '')::text[] AS regions,
    array_agg(DISTINCT t.val) FILTER (WHERE t.kind = 'y' AND t.val <> '')::text[] AS cities
FROM member m
JOIN jobs o ON o.id = m.member_id
LEFT JOIN LATERAL (
    SELECT 'c'::text AS kind, e AS val FROM unnest(o.countries) AS e
    UNION ALL
    SELECT 'r', e FROM unnest(o.regions) AS e
    UNION ALL
    SELECT 'y', e FROM unnest(o.cities) AS e
) t ON true
GROUP BY m.owner_id;

-- name: DuplicateClosureGeoFor :many
-- The duplicate-closure geography union for a SPECIFIC set of owner ids, so an incremental
-- index push can widen a whole wave's rows in one query instead of one call per job.
--
-- The recursive body below is a COPY of DuplicateClosureGeoAll's, not a shared one — sqlc
-- names whole statements, so there is nowhere to put it once. The walk, the open-rows-only
-- scope, the closed-intermediate behaviour and the depth bound are argued there. What must
-- stay identical is the recursive term and the seed's `closed_at IS NULL AND duplicate_of IS
-- NULL`: change either here alone and the wave's answer stops matching the rebuild's, which
-- surfaces as a canon that silently narrows every time the drain touches it.
--
-- One deliberate difference: the seed carries no EXISTS test. The caller named these rows, and
-- a row representing nobody answers with its own geography — a self-union, and a documented
-- no-op merge. That keeps the caller to one error branch instead of making it tell "this row
-- owns nothing" apart from a failure.
--
-- An id matching no open canonical row simply yields no row for that id, which the caller
-- reads as "no widening".
WITH RECURSIVE member AS (
    SELECT o.id AS owner_id, o.id AS member_id, 0 AS depth
    FROM jobs o
    WHERE o.id = ANY(sqlc.arg(owner_ids)::bigint[])
      AND o.closed_at IS NULL
      AND o.duplicate_of IS NULL
    UNION ALL
    SELECT m.owner_id, c.id, m.depth + 1
    FROM member m
    JOIN jobs c ON c.duplicate_of = m.member_id
    WHERE c.closed_at IS NULL AND m.depth < 8
)
SELECT
    m.owner_id,
    array_agg(DISTINCT t.val) FILTER (WHERE t.kind = 'c' AND t.val <> '')::text[] AS countries,
    array_agg(DISTINCT t.val) FILTER (WHERE t.kind = 'r' AND t.val <> '')::text[] AS regions,
    array_agg(DISTINCT t.val) FILTER (WHERE t.kind = 'y' AND t.val <> '')::text[] AS cities
FROM member m
JOIN jobs o ON o.id = m.member_id
LEFT JOIN LATERAL (
    SELECT 'c'::text AS kind, e AS val FROM unnest(o.countries) AS e
    UNION ALL
    SELECT 'r', e FROM unnest(o.regions) AS e
    UNION ALL
    SELECT 'y', e FROM unnest(o.cities) AS e
) t ON true
GROUP BY m.owner_id;

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

-- name: RecomputeRoleDuplicatesForCompanies :one
-- The batched slice of the role-duplicate recompute, driven over a CHUNK of companies
-- (cmd/reindex's forCompanyBatches) rather than one call per company — see that
-- function's doc comment for why: at catalogue scale (2026-08-06 prod measurement:
-- 236,923 distinct open companies) one round trip per company made this pass take
-- hours under ordinary host load, most of it network/planning overhead rather than
-- query cost. Batching multiple companies into ONE statement is safe here without any
-- extra cross-company guard: role_fingerprint is sha256(company_slug, title,
-- description) (internal/job/jobhash.RoleFingerprint) with company_slug as its FIRST
-- component, so a fingerprint collision between two different companies is not a
-- realistic concern — grouping by role_fingerprint alone, across the whole batch,
-- cannot merge two different companies' rows. Canon = min(id) among a role's open rows;
-- the canon and any singleton/empty-fp row get duplicate_of NULL, the other reposts
-- point to the canon. Rows are never deleted, so the reality counts are untouched. The
-- (company_slug, role_fingerprint) index makes both scans range scans over the batch.
-- The IS DISTINCT FROM guard makes re-runs cheap and idempotent, and a closed canon
-- fails over to the next min(id) on the next run.
WITH canon AS (
    SELECT jobs.role_fingerprint, MIN(jobs.id) AS canon_id, COUNT(*) AS n
    FROM jobs
    WHERE jobs.company_slug = ANY(sqlc.arg(companies)::text[])
      AND jobs.closed_at IS NULL AND jobs.role_fingerprint IS NOT NULL AND jobs.role_fingerprint <> ''
    GROUP BY jobs.role_fingerprint
),
target AS (
    SELECT j.id,
        CASE WHEN c.n > 1 AND j.id <> c.canon_id THEN c.canon_id END AS new_dup,
        j.duplicate_of AS was_duplicate_of
    FROM jobs j
    JOIN canon c ON j.role_fingerprint = c.role_fingerprint
    WHERE j.company_slug = ANY(sqlc.arg(companies)::text[]) AND j.closed_at IS NULL
),
updated AS (
    UPDATE jobs j
    SET duplicate_of_role = t.new_dup,
        updated_at        = now()
    FROM target t
    WHERE j.id = t.id
      AND j.duplicate_of_role IS DISTINCT FROM t.new_dup
    RETURNING j.id,
              t.was_duplicate_of,
              j.duplicate_of AS now_duplicate_of,
              COALESCE(j.posted_at, j.created_at) AS eff_posted_at
),
dequeued AS (
    INSERT INTO search_delete_outbox (job_id)
    SELECT id FROM updated WHERE was_duplicate_of IS NULL AND now_duplicate_of IS NOT NULL
    ON CONFLICT (job_id) DO NOTHING
),
requeued AS (
    INSERT INTO search_outbox (job_id, job_posted_at)
    SELECT id, eff_posted_at FROM updated WHERE was_duplicate_of IS NOT NULL AND now_duplicate_of IS NULL
    ON CONFLICT (job_id) DO NOTHING
)
SELECT count(*)::bigint FROM updated;

-- name: CompaniesWithAggregatorPostings :many
-- Company slugs with at least one OPEN aggregator posting — the drive list for the
-- cross-source aggregator suppression pass. An open aggregator row is a candidate whether
-- it still needs suppressing OR needs releasing (its ATS twin closed), so one predicate
-- covers both. Processed in chunks (SuppressAggregatorDuplicatesForCompanies), mirroring
-- the role-duplicate recompute's batching.
SELECT DISTINCT company_slug FROM jobs
WHERE closed_at IS NULL AND company_slug <> ''
  AND source = ANY(sqlc.arg(aggregators)::text[]);

-- name: SuppressAggregatorDuplicatesForCompanies :one
-- The batched slice of the cross-source aggregator suppression, driven over a CHUNK of
-- companies (cmd/reindex's forCompanyBatches) rather than one call per company — see
-- RecomputeRoleDuplicatesForCompanies' doc comment for the prod measurement that
-- motivated batching both passes (94,410 companies with an open aggregator posting;
-- one round trip each made this pass the one that actually got stuck for hours on
-- 2026-08-06). Unlike that query, batching here is NOT free: title matching has no
-- natural company key the way role_fingerprint does, so both CTEs carry an explicit
-- fcompany (folded company) column and every match arm's ON clause pins
-- t.fcompany = a.fcompany — without it, two different companies sharing a common title
-- ("Backend Engineer") would cross-match the moment they land in the same batch.
-- fcompany is the same replace(company_slug, '-', '') fold PR #1591 introduced (a
-- source spelling one employer "Cfoinsights", another "CFO Insights" — different
-- slugs that must still agree), just computed once per row instead of per query.
--
-- The batch arrives ALREADY FOLDED (cmd/reindex's foldCompanySlugs) and is compared
-- against the STORED jobs.company_slug_folded column, not against
-- `replace(company_slug,'-','')`. That distinction is the whole reason this pass is
-- finishable. Against the expression the planner has no usable selectivity estimate
-- once the values arrive as a parameter: measured on prod 2026-08-16 it expected 1.4M
-- rows and got 734, drove each batch off the SOURCE index, and read ~927k aggregator
-- rows per batch of 500 companies — 271s each, ~23h over the 306 batches, against a
-- 12h unit timeout the run never survived (0 successful reindexes in 3 days).
--
-- Rewriting the query does not help (array parameter 259s, JOIN over unnest 315s,
-- LATERAL per company 300s), and neither does more statistics (raising the functional
-- index's target moved n_distinct 16,817 -> 147,101, query unchanged at 298s). A plain
-- column does: the same predicate shape over the existing company_slug column measured
-- 491ms. See migrations/0109 for why the column is maintained by the write paths
-- rather than GENERATED, and folded_slug_rule_test.go for the test that keeps them
-- honest.
--
-- A row whose folded column is still NULL (the backfill is chunked and online) simply
-- does not match, so the pass suppresses less until it completes — never wrongly.
--
-- An open aggregator posting is marked duplicate_of an open CANONICAL ATS
-- (non-aggregator) posting of the same (folded) company, equal normalized title, and
-- compatible country (countries overlap, or either side empty — the geography
-- dictionary is sparse, so an unresolved side must not veto). The ATS row is never
-- touched, so it stays canonical. Candidate aggregator rows are those that are
-- canonical OR already point at a non-aggregator row (i.e. suppressed by THIS pass) —
-- an aggregator repost pointed at another aggregator by the role pass is left alone. A
-- candidate with no ATS twin resolves to NULL, so a closed twin releases its aggregator
-- copy back into search/embedding/enrichment. min(id) picks a stable target; the IS
-- DISTINCT FROM guard makes re-runs cheap and idempotent. Run AFTER
-- RecomputeRoleDuplicatesForCompanies so ATS reposts have already collapsed to their canon.
WITH ats AS (
    SELECT jobs.id,
           jobs.company_slug_folded AS fcompany,
           btrim(regexp_replace(lower(jobs.title), '[^a-z0-9]+', ' ', 'g')) AS ntitle,
           btrim(regexp_replace(lower(
             regexp_replace(
               regexp_replace(
                 regexp_replace(jobs.title, '&[a-zA-Z0-9#]+;', ' ', 'g'),
                 '^(.*?):.+$', '\1'),
               '^(.*)\s[-|—]\s.+$', '\1')
           ), '[^a-z0-9]+', ' ', 'g')) AS ntitle2,
           jobs.countries
    FROM jobs
    WHERE jobs.company_slug_folded = ANY(sqlc.arg(folded_companies)::text[])
      AND jobs.closed_at IS NULL AND jobs.duplicate_of IS NULL AND jobs.company_slug <> ''
      AND NOT (jobs.source = ANY(sqlc.arg(aggregators)::text[]))
),
agg AS (
    SELECT a.id,
           a.company_slug_folded AS fcompany,
           btrim(regexp_replace(lower(a.title), '[^a-z0-9]+', ' ', 'g')) AS ntitle,
           btrim(regexp_replace(lower(
             regexp_replace(
               regexp_replace(
                 regexp_replace(a.title, '&[a-zA-Z0-9#]+;', ' ', 'g'),
                 '^(.*?):.+$', '\1'),
               '^(.*)\s[-|—]\s.+$', '\1')
           ), '[^a-z0-9]+', ' ', 'g')) AS ntitle2,
           a.countries
    FROM jobs a
    WHERE a.company_slug_folded = ANY(sqlc.arg(folded_companies)::text[])
      AND a.closed_at IS NULL AND a.company_slug <> ''
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
matches AS (
    -- Two match paths: the exact key (ntitle) and the entity-decoded, suffix-stripped key
    -- (ntitle2), which catches a title that only appends a trailing " - <suffix>" or
    -- ": <suffix>" clause, or carries an undecoded HTML entity. The colon clause is stripped
    -- INSIDE the dash strip so a title carrying both ("Engineer: Go, K8s - Remote") reduces all
    -- the way. Only those two clauses are decoration: measured on prod, also stripping a
    -- parenthetical produced 39 wrong pairs out of 55 — one company's "…, Backend (Traffic)"
    -- matching its "(Payments)", "(Identity)" and "(Infrastructure)" roles — and a comma clause
    -- fails the same way ("…, Backend" vs "…, Fullstack"). Each path is a SEPARATE
    -- equality hash join (now on (fcompany, ntitle) / (fcompany, ntitle2), still
    -- O(agg + ats)) and the two are UNION ALL-ed — an OR of the two equalities in one ON
    -- would defeat the hash join and go quadratic on a big company (the hotel-chain
    -- case). UNION ALL, not UNION: a row matched by both paths appears twice, but the
    -- downstream MIN(ats_id) absorbs the duplicate, so the de-dup pass is wasted work.
    -- Both require a non-empty key; the country gate applies to each path.
    SELECT a.id AS agg_id, t.id AS ats_id
    FROM agg a JOIN ats t
      ON t.fcompany = a.fcompany
     AND t.ntitle = a.ntitle AND a.ntitle <> ''
     AND (t.countries && a.countries OR cardinality(t.countries) = 0 OR cardinality(a.countries) = 0)
    UNION ALL
    SELECT a.id, t.id
    FROM agg a JOIN ats t
      ON t.fcompany = a.fcompany
     AND t.ntitle2 = a.ntitle2 AND a.ntitle2 <> ''
     AND (t.countries && a.countries OR cardinality(t.countries) = 0 OR cardinality(a.countries) = 0)
    UNION ALL
    -- Third path: word-subset containment — the aggregator dropped words the ATS keeps (a
    -- mid-title drop the two equality keys miss). This arm is a nested loop (no hash on <@),
    -- but runs only on the residual after the equality arms and is bounded per company via
    -- the fcompany equality (planned as a hash/merge join on fcompany, nested-looping only
    -- within each matching group — the same cost shape the old single-company call had).
    -- Guards against over-merge: the aggregator title needs >= 2 words, and the words the ATS
    -- adds over it must include at least one NON-seniority word — so "Software Engineer" is not
    -- merged into "Senior Software Engineer" (a distinct grade), only into a title that adds a
    -- real specialty/location/department word the aggregator dropped.
    SELECT a.id, t.id
    FROM agg a JOIN ats t
      ON t.fcompany = a.fcompany
     AND string_to_array(a.ntitle, ' ') <@ string_to_array(t.ntitle, ' ')
     AND array_length(string_to_array(a.ntitle, ' '), 1) >= 2
     AND (t.countries && a.countries OR cardinality(t.countries) = 0 OR cardinality(a.countries) = 0)
     AND EXISTS (
         SELECT 1 FROM unnest(string_to_array(t.ntitle, ' ')) AS w
         WHERE w <> ALL (string_to_array(a.ntitle, ' '))
           AND w <> ALL (ARRAY['senior','sr','junior','jr','lead','principal','staff','mid',
                               'midlevel','entry','chief','intern','trainee','graduate',
                               'apprentice','ii','iii','iv']::text[])
     )
),
target AS (
    -- Every candidate aggregator row, with its MIN matching ATS id or NULL. LEFT JOIN so an
    -- unmatched row (including one whose ATS twin just closed) resolves to NULL and is
    -- released back into search/embedding/enrichment. min(id) picks a stable target.
    SELECT a.id, MIN(m.ats_id) AS new_dup,
           MIN(j.duplicate_of) AS was_duplicate_of
    FROM agg a
    LEFT JOIN matches m ON m.agg_id = a.id
    JOIN jobs j ON j.id = a.id
    GROUP BY a.id
),
updated AS (
    UPDATE jobs j
    SET duplicate_of_aggregator = t.new_dup,
        updated_at              = now()
    FROM target t
    WHERE j.id = t.id
      AND j.duplicate_of_aggregator IS DISTINCT FROM t.new_dup
    RETURNING j.id,
              t.was_duplicate_of,
              j.duplicate_of AS now_duplicate_of,
              COALESCE(j.posted_at, j.created_at) AS eff_posted_at
),
dequeued AS (
    INSERT INTO search_delete_outbox (job_id)
    SELECT id FROM updated WHERE was_duplicate_of IS NULL AND now_duplicate_of IS NOT NULL
    ON CONFLICT (job_id) DO NOTHING
),
requeued AS (
    INSERT INTO search_outbox (job_id, job_posted_at)
    SELECT id, eff_posted_at FROM updated WHERE was_duplicate_of IS NOT NULL AND now_duplicate_of IS NULL
    ON CONFLICT (job_id) DO NOTHING
)
SELECT count(*)::bigint FROM updated;

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
    public_slug, countries, regions, cities, work_mode, skills, seniority, category, is_tech, requires_clearance,
    posting_language, employment_type, education_level, english_level, experience_years_min,
    salary_min_manual, salary_max_manual, salary_currency_manual, salary_period_manual, enrichment,
    content_hash, role_fingerprint,
    created_by
) VALUES (
    sqlc.arg(source), sqlc.arg(external_id), sqlc.arg(url), sqlc.arg(title),
    sqlc.arg(company), sqlc.arg(company_slug), replace(sqlc.arg(company_slug), '-', ''), sqlc.arg(location), sqlc.arg(remote),
    sqlc.arg(description), sqlc.arg(posted_at),
    sqlc.arg(public_slug),
    COALESCE(sqlc.arg(countries)::text[], '{}'), COALESCE(sqlc.arg(regions)::text[], '{}'), COALESCE(sqlc.arg(cities)::text[], '{}'),
    sqlc.arg(work_mode), COALESCE(sqlc.arg(skills)::text[], '{}'),
    sqlc.arg(seniority), sqlc.arg(category), sqlc.arg(is_tech), sqlc.arg(requires_clearance),
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
    work_mode    = EXCLUDED.work_mode,
    skills       = EXCLUDED.skills,
    seniority    = EXCLUDED.seniority,
    category     = EXCLUDED.category,
    is_tech      = EXCLUDED.is_tech,
    requires_clearance = EXCLUDED.requires_clearance,
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
    -- Same reasoning as UpsertJob: a moderator company correction invalidates this
    -- job's already-precomputed similar-jobs list (it may have been computed
    -- excluding the OLD company), so force cmd/similar-backfill to redo it —
    -- conditionally, since every re-create runs this branch, not just company edits.
    similar_computed_at = CASE WHEN jobs.company_slug IS DISTINCT FROM EXCLUDED.company_slug
                               THEN NULL ELSE jobs.similar_computed_at END,
    updated_at   = now()
RETURNING *;

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
    public_slug, countries, regions, cities, work_mode, skills, seniority, category, is_tech, requires_clearance,
    posting_language, employment_type, education_level, english_level, experience_years_min,
    content_hash, role_fingerprint,
    created_by, is_private
) VALUES (
    sqlc.arg(source), sqlc.arg(external_id), sqlc.arg(url), sqlc.arg(title),
    sqlc.arg(company), sqlc.arg(company_slug), replace(sqlc.arg(company_slug), '-', ''), sqlc.arg(location), sqlc.arg(remote),
    sqlc.arg(description),
    sqlc.arg(public_slug),
    COALESCE(sqlc.arg(countries)::text[], '{}'), COALESCE(sqlc.arg(regions)::text[], '{}'), COALESCE(sqlc.arg(cities)::text[], '{}'),
    sqlc.arg(work_mode), COALESCE(sqlc.arg(skills)::text[], '{}'),
    sqlc.arg(seniority), sqlc.arg(category), sqlc.arg(is_tech), sqlc.arg(requires_clearance),
    sqlc.arg(posting_language), sqlc.arg(employment_type), sqlc.arg(education_level), sqlc.arg(english_level), sqlc.arg(experience_years_min),
    sqlc.arg(content_hash), sqlc.arg(role_fingerprint),
    sqlc.arg(created_by)::bigint, true
)
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
    company_slug_folded = replace(sqlc.arg(company_slug), '-', ''),
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
    is_tech      = sqlc.arg(is_tech),
    requires_clearance = sqlc.arg(requires_clearance),
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
    updated_by   = sqlc.arg(updated_by)::bigint,
    -- Same reasoning as UpsertJob: a company correction invalidates this job's
    -- already-precomputed similar-jobs list (it may have been computed excluding the
    -- OLD company), so force cmd/similar-backfill to redo it — conditionally, since
    -- most edits leave the company untouched.
    similar_computed_at = CASE WHEN jobs.company_slug IS DISTINCT FROM sqlc.arg(company_slug)
                               THEN NULL ELSE jobs.similar_computed_at END,
    updated_at   = now()
WHERE public_slug = sqlc.arg(public_slug) AND created_by IS NOT NULL
RETURNING *;

-- name: CloseUnseenJobs :one
-- Post-ingest sweep (see job-lifecycle spec): close every open job of ONE source not
-- seen since the cutoff, scoped to the company slugs the run actually crawled. Scoped
-- by source because ingest runs per provider (a greenhouse run must not close jobs
-- another provider owns), and by company_slug because a run may crawl only a SUBSET of
-- a provider's boards — a partial or targeted run (or a full crawl of a huge provider
-- that times out and only completes some boards) must not close the companies it never
-- touched. The caller passes the crawled slugs and owns the grace window (cutoff =
-- now() - window), so neither a failed nor a partial crawl mass-closes a catalogue.
--
-- The removal enqueue rides this statement rather than being a call per closed row.
-- A sweep closes a whole provider's stale postings in one round trip, so anything
-- per-row would undo that; feeding search_delete_outbox from the UPDATE's own RETURNING
-- keeps the enqueue atomic with the close (a rolled-back sweep queues nothing) and
-- exact (only rows that actually closed are queued).
--
-- :one rather than :execrows because the CTE moves the row count out of the command tag.
-- count(*) over the closed rows is the same int64 the caller already had, so no call site
-- changes.
WITH closed AS (
    UPDATE jobs
    SET closed_at     = now(),
        closed_reason = 'unseen',
        updated_at    = now()
    WHERE closed_at IS NULL
      AND source = sqlc.arg(source)
      AND last_seen_at < sqlc.arg(cutoff)
      AND company_slug = ANY(sqlc.arg(company_slugs)::text[])
    RETURNING id
), queued AS (
    INSERT INTO search_delete_outbox (job_id)
    SELECT id FROM closed
    ON CONFLICT (job_id) DO NOTHING
)
SELECT count(*) FROM closed;

-- name: UnseenJobIDs :many
-- Same candidate set as CloseUnseenJobs, unmaterialized. The sweep's fallback path
-- (see CloseUnseenJobByID) uses this to close row by row when the single bulk UPDATE
-- fails — e.g. a heap/index-corrupted row aborts the whole batch (2026-08-11 incident:
-- one duplicated jobs_pkey value blocked greenhouse's sweep on every run) — so ids are
-- fetched separately and closed one at a time, letting one bad id be skipped without
-- blocking the rest.
SELECT id FROM jobs
WHERE closed_at IS NULL
  AND source = sqlc.arg(source)
  AND last_seen_at < sqlc.arg(cutoff)
  AND company_slug = ANY(sqlc.arg(company_slugs)::text[]);

-- name: UnseenJobIDsBySource :many
-- Row-by-row fallback candidate set for CloseUnseenJobsBySource — see UnseenJobIDs.
SELECT id FROM jobs
WHERE closed_at IS NULL
  AND source = sqlc.arg(source)
  AND last_seen_at < sqlc.arg(cutoff);

-- name: CloseUnseenJobByID :one
--
-- The removal enqueue rides this statement (see CloseUnseenJobs for why): feeding
-- search_delete_outbox from the UPDATE's own RETURNING keeps it atomic with the close and
-- exact — only rows that actually closed are queued, and a rolled-back close queues nothing.
--
-- :one rather than :execrows because the CTE moves the row count out of the command tag.
-- count(*) over the closed rows is the same int64 the caller already had.
-- Row-by-row sweep fallback (see UnseenJobIDs): closes with the same 'unseen' reason
-- as the bulk sweep, one id at a time, so a single row's error (e.g. corrupted index
-- entry) can be caught and skipped by the caller without losing the rest of the batch.
WITH closed AS (
    UPDATE jobs
    SET closed_at     = now(),
        closed_reason = 'unseen',
        updated_at    = now()
    WHERE jobs.id = sqlc.arg(id) AND jobs.closed_at IS NULL
    RETURNING jobs.id
), queued AS (
    INSERT INTO search_delete_outbox (job_id)
    SELECT id FROM closed
    ON CONFLICT (job_id) DO NOTHING
)
SELECT count(*) FROM closed;

-- name: CloseUnseenJobsBySource :one
--
-- The removal enqueue rides this statement (see CloseUnseenJobs for why): feeding
-- search_delete_outbox from the UPDATE's own RETURNING keeps it atomic with the close and
-- exact — only rows that actually closed are queued, and a rolled-back close queues nothing.
--
-- :one rather than :execrows because the CTE moves the row count out of the command tag.
-- count(*) over the closed rows is the same int64 the caller already had.
-- Post-ingest sweep for a fullCatalog source (see job-lifecycle spec): close every open job of
-- ONE source not seen since the cutoff, WITHOUT the crawled-company scope. A fullCatalog adapter
-- (e.g. habr_career) lists its whole catalogue each run, so an unseen job is genuinely gone —
-- including the last posting of a company that dropped out of the feed entirely, which the
-- company-scoped CloseUnseenJobs cannot reach. cmd/ingest calls this ONLY after a zero-Failed run
-- of a fullCatalog provider (a truncated crawl, which such adapters surface as an error, would
-- otherwise mass-close everything it never reached); a partial run falls back to CloseUnseenJobs.
WITH closed AS (
    UPDATE jobs
    SET closed_at     = now(),
        closed_reason = 'unseen',
        updated_at    = now()
    WHERE closed_at IS NULL
      AND source = sqlc.arg(source)
      AND last_seen_at < sqlc.arg(cutoff)
    RETURNING id
), queued AS (
    INSERT INTO search_delete_outbox (job_id)
    SELECT id FROM closed
    ON CONFLICT (job_id) DO NOTHING
)
SELECT count(*) FROM closed;

-- name: CloseJobBySourceExternalID :one
--
-- The removal enqueue rides this statement (see CloseUnseenJobs for why): feeding
-- search_delete_outbox from the UPDATE's own RETURNING keeps it atomic with the close and
-- exact — only rows that actually closed are queued, and a rolled-back close queues nothing.
--
-- :one rather than :execrows because the CTE moves the row count out of the command tag.
-- count(*) over the closed rows is the same int64 the caller already had.
-- Stream-driven close (see job-lifecycle): a self-closing feed source (e.g. jobtech)
-- learns of a removed posting from its incremental stream and closes it by identity,
-- rather than relying on the post-run unseen sweep (which it opts out of, since an
-- incremental stream re-reports only changed ads and so never refreshes last_seen_at
-- for the still-open ones). WHERE closed_at IS NULL keeps it idempotent; a later
-- upsert of the same (source, external_id) reopens it if the posting reappears.
WITH closed AS (
    UPDATE jobs
    SET closed_at     = now(),
        closed_reason = 'feed_removed',
        updated_at    = now()
    WHERE closed_at IS NULL
      AND source = sqlc.arg(source)
      AND external_id = sqlc.arg(external_id)
    RETURNING id
), queued AS (
    INSERT INTO search_delete_outbox (job_id)
    SELECT id FROM closed
    ON CONFLICT (job_id) DO NOTHING
)
SELECT count(*) FROM closed;

-- name: ExistingExternalIDs :many
-- Seen-set for a hydrating source (see source-ingest): all external_ids stored for one
-- provider, so an adapter with expensive per-posting detail (justjoin, ~20k live offers)
-- fetches detail only for postings the catalogue does not already have. Closed rows are
-- included — a closed posting is still "seen" (no need to re-fetch its detail; a reappearance
-- reopens it via the upsert regardless). Keyed by source alone; the caller namespaces the
-- adapter's raw posting id to match the stored external_id.
--
-- is_tech rides along because a hydrating crawl re-lists a posting without its description: it is
-- the evidence the catalogue filter reads, and only the stored row still has it.
--
-- A row with NO description is not fully ingested — its one detail fetch failed and, because
-- being stored is what makes a posting "seen", nothing would ever retry it: the body would be
-- missing for the row's whole life (freehire#1866 found ~3.3k such live rows across the
-- hydrating sources). Such a row is therefore withheld from the seen-set until
-- hydration_cutoff, which re-offers it for detail exactly as if it were new; past the cutoff it
-- counts as seen again, so a posting the source genuinely publishes with no body stops costing
-- a detail request every crawl forever.
SELECT external_id, is_tech FROM jobs
WHERE source = sqlc.arg(source)
  AND (description <> '' OR created_at < sqlc.arg(hydration_cutoff));

-- name: ExistingExternalIDsByBoard :many
-- Seen-set of ONE board of a multi-board provider. The lookup runs once per crawled board, so a
-- provider-wide read is unaffordable where the provider is large: on workday it returns 1.27M ids
-- in ~168s, against ~1.8s for a board's own 25k.
--
-- Matched as a LIKE prefix so it rides jobs_source_extid_pattern_idx (source, external_id
-- text_pattern_ops), whose operator class compares byte-wise. A range predicate over the plain
-- index (external_id >= 'board:' AND < 'board;') looks equivalent and is NOT: under the database's
-- collation punctuation carries only a secondary weight, so that range returns nothing at all.
-- The caller passes an escaped pattern (externalid.BoardPattern) — a board name may contain LIKE
-- syntax, and an unescaped underscore would match a sibling board.
--
-- hydration_cutoff withholds a still-body-less row from the seen-set so its detail is retried;
-- see ExistingExternalIDs for why.
SELECT external_id, is_tech FROM jobs
WHERE source = sqlc.arg(source)
  AND external_id LIKE sqlc.arg(pattern)
  AND (description <> '' OR created_at < sqlc.arg(hydration_cutoff));

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
    closed_reason = '',
    -- A reopen resets the strike count so a single later expired probe can't immediately
    -- re-close it, mirroring UpsertJob.
    liveness_strikes = CASE WHEN closed_at IS NOT NULL THEN 0 ELSE liveness_strikes END,
    -- updated_at moves ONLY on a reopen. A re-listing that changed nothing is not a content
    -- change, and stamping it would keep the column meaning "last crawled" — the same economy
    -- RefreshUnchangedJob brings to the board path, applied to the hydrating one. A reopen IS a
    -- change the search reconciler must see, so that case still stamps. The RHS reads the row's
    -- pre-update closed_at, which is why one CASE serves both this and the strike reset above.
    updated_at   = CASE WHEN closed_at IS NOT NULL THEN now() ELSE updated_at END
WHERE source = sqlc.arg(source) AND external_id = sqlc.arg(external_id)
RETURNING company_slug;

-- name: CloseJobByID :one
--
-- The removal enqueue rides this statement (see CloseUnseenJobs for why): feeding
-- search_delete_outbox from the UPDATE's own RETURNING keeps it atomic with the close and
-- exact — only rows that actually closed are queued, and a rolled-back close queues nothing.
--
-- :one rather than :execrows because the CTE moves the row count out of the command tag.
-- count(*) over the closed rows is the same int64 the caller already had.
-- Soft-close one job now (see job-lifecycle): a moderator resolving a report with
-- close_job=true. The third writer of closed_at, alongside the ingest sweep and the
-- liveness probe. WHERE closed_at IS NULL keeps it idempotent — a second close on an
-- already-closed job is a no-op, never an error, so it never fights the report's own
-- status guard. A later ingest upsert may legitimately reopen a board job (reopen-on-
-- reappear); that is the lifecycle's existing behavior, not a conflict.
WITH closed AS (
    UPDATE jobs
    SET closed_at     = now(),
        closed_reason = 'moderated',
        updated_at    = now()
    WHERE jobs.id = sqlc.arg(id) AND jobs.closed_at IS NULL
    RETURNING jobs.id
), queued AS (
    INSERT INTO search_delete_outbox (job_id)
    SELECT id FROM closed
    ON CONFLICT (job_id) DO NOTHING
)
SELECT count(*) FROM closed;

-- name: CloseStaleUnsignalledJobs :execrows
-- Age rule (see job-lifecycle): close open jobs from the sources that carry NO close
-- signal at all — no re-crawl that could stop seeing them, no change feed, and no posting
-- URL a probe could reach a verdict on. Today that is exactly `telegram`, whose stored URL
-- is the post itself and outlives the vacancy (cmd/liveness's unsignalledSources).
--
-- This is the only close that rests on a guess rather than on evidence, which is why it is
-- scoped by an explicit source list the CALLER passes rather than by "everything the sweep
-- misses": a source must be opted in deliberately, so a new adapter can never drift into
-- being closed by age. The caller also owns the window (cutoff = now() - window), the same
-- division of labour the unseen sweep uses.
--
-- The ANY() form fails CLOSED on an empty or nil list: `source = ANY('{}')` is FALSE for
-- every row (and ANY(NULL) is NULL), so a caller that passes nothing closes nothing. That is
-- the exact mirror of SelectOrphanLivenessCandidates, where `<> ALL('{}')` is vacuously TRUE
-- and therefore needs a guard. Do not invert this predicate without restoring one.
--
-- Strictly older than the cutoff, so a row exactly at the boundary survives one more run —
-- under-closing is the correct bias when there is no evidence to appeal to. Idempotent via
-- WHERE closed_at IS NULL: a cron worker runs this repeatedly and closes each row once.
UPDATE jobs
SET closed_at     = now(),
    closed_reason = 'expired',
    updated_at    = now()
WHERE closed_at IS NULL
  AND source = ANY(sqlc.arg(sources)::text[])
  AND COALESCE(posted_at, created_at) < sqlc.arg(cutoff);

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

-- name: SelectStaleRegisteredCandidates :many
-- Liveness backstop for a registered provider whose ingest sweep cannot reach every open
-- job — see job-lifecycle: CloseUnseenJobs scopes closes to the company_slugs a run
-- actually crawled, so a company that ages out of a recency-budgeted aggregator's crawl
-- window (himalayas pages only its freshest slice) never re-enters that scope and its
-- last posting leaks open forever. Unlike SelectOrphanLivenessCandidates (any job whose
-- source ISN'T swept), this targets specific sources that ARE swept but only jobs the
-- sweep already should have closed by its own 48h window (cmd/ingest's staleAfter) —
-- evidence the sweep is structurally unable to reach them, not a race with it.
-- external_id rides along for sources verified by a per-posting API keyed on it rather
-- than by fetching the stored url (echojobs: see cmd/liveness/echojobs.go).
SELECT id, source, url, external_id, public_slug, liveness_strikes
FROM jobs
WHERE closed_at IS NULL
  AND source = ANY(sqlc.arg(sources)::text[])
  AND last_seen_at < sqlc.arg(cutoff);

-- name: MarkLivenessExpired :one
-- Record one expired probe: increment the strike counter and, in the same write,
-- close the job (closed_at) once it reaches the threshold the caller owns — the
-- two-strike grace that absorbs a transient death signal. Returns the new strike
-- count and closed_at so the worker can log the outcome.
UPDATE jobs
SET liveness_strikes = liveness_strikes + 1,
    -- AND closed_at IS NULL on both branches below: if another mechanism closed this row
    -- between candidate selection and this write, its closed_at/closed_reason are the true
    -- record of when and why, and this probe must not overwrite either.
    closed_at = CASE
        WHEN liveness_strikes + 1 >= sqlc.arg(threshold) AND closed_at IS NULL THEN now()
        ELSE closed_at
    END,
    closed_reason = CASE
        WHEN liveness_strikes + 1 >= sqlc.arg(threshold) AND closed_at IS NULL THEN 'probe_expired'
        ELSE closed_reason
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
-- seniority, category, is_tech, requires_clearance, plus the synthetic enrichment
-- facets posting_language,
-- employment_type, education_level, english_level, experience_years_min, all from
-- jobderive.Derive), the repost-identity role_fingerprint (internal/job/jobhash), and the
-- public_slug/company_slug (internal/dict/normalize). One keyset scan propagates any
-- dictionary/algorithm change to old and closed rows that never re-crawl. Every column
-- is a pure function of the raw fields, so the write is idempotent. COALESCE maps a nil
-- array arg to '{}' for the NOT NULL array columns; work_mode is written as given by the
-- caller, which preserves an already-set (possibly adapter-structured) value.
--
-- updated_at is bumped ONLY when role_fingerprint actually moves (the SET clause reads
-- the pre-update row, so the guard compares the stored fingerprint to the new one): a
-- fingerprint change must reach `reindex --since`, which recomputes duplicate_of, while a
-- facet/slug-only rewrite deliberately leaves the timestamp untouched so a big backfill
-- does not churn every row's updated_at.
UPDATE jobs
SET countries = COALESCE(sqlc.arg(countries)::text[], '{}'),
    regions   = COALESCE(sqlc.arg(regions)::text[], '{}'),
    cities    = COALESCE(sqlc.arg(cities)::text[], '{}'),
    work_mode = sqlc.arg(work_mode),
    skills    = COALESCE(sqlc.arg(skills)::text[], '{}'),
    seniority = sqlc.arg(seniority),
    category  = sqlc.arg(category),
    is_tech   = sqlc.arg(is_tech),
    requires_clearance = sqlc.arg(requires_clearance),
    posting_language     = sqlc.arg(posting_language),
    employment_type      = sqlc.arg(employment_type),
    education_level      = sqlc.arg(education_level),
    english_level        = sqlc.arg(english_level),
    experience_years_min = sqlc.arg(experience_years_min),
    role_fingerprint = sqlc.arg(role_fingerprint),
    public_slug      = sqlc.arg(public_slug),
    company_slug     = sqlc.arg(company_slug),
    company_slug_folded = replace(sqlc.arg(company_slug), '-', ''),
    updated_at = CASE
        WHEN role_fingerprint IS DISTINCT FROM sqlc.arg(role_fingerprint) THEN now()
        ELSE updated_at
    END
WHERE id = sqlc.arg(id);

-- name: RoleClusterCountsFor :many
-- Role-cluster counts for a SPECIFIC set of (company_slug, role_fingerprint) pairs, so
-- a read path can resolve a page of cards in one query instead of one per card.
--
-- RoleClusterCountsAll exists beside this and is not a substitute: it aggregates the
-- whole catalogue, which is right for a reindex building its lookup once and ruinous
-- for a request. Filtering on role_fingerprint alone would not do either, since
-- jobs_company_role_fingerprint_idx leads with company_slug; leading with the company
-- set keeps the index usable.
--
-- The two sets are matched as a cross product here and narrowed to the exact pairs by
-- the caller. A pair-wise join would need a two-argument unnest the query analyzer
-- cannot type, and the surplus is bounded: a page holds few distinct companies, and a
-- fingerprint belonging to another of them simply has no rows.
SELECT j.company_slug,
       j.role_fingerprint,
       COUNT(*)::bigint AS repost_count,
       COUNT(*) FILTER (WHERE j.closed_at IS NULL)::bigint AS mass_count
FROM jobs j
WHERE j.company_slug = ANY(sqlc.arg(company_slugs)::text[])
  AND j.role_fingerprint = ANY(sqlc.arg(role_fingerprints)::text[])
GROUP BY j.company_slug, j.role_fingerprint;

-- name: CompaniesWithFuzzyDedupCandidates :many
-- Company slugs worth running the fuzzy-description pass over: a company that still has more
-- than one open CANONICAL posting after the exact role-cluster and aggregator passes, so there
-- is something left that byte-exact matching did not collapse. Rows without a company_slug are
-- excluded: the pass buckets by (company, title) and relies on that bucket to keep unrelated
-- roles apart, and an empty slug is not a company boundary — measured on prod, 105 212 such rows
-- fall into 20 126 same-title buckets spanning up to four different employers.
-- The pass then processes these ONE COMPANY AT A TIME, like the other duplicate passes, so it
-- never holds a lock wide enough to stall a concurrent ingest crawl.
--
-- The predicate names the OWNED columns of the exact passes, not the derived duplicate_of. Those
-- are not the same set: duplicate_of also carries this pass's OWN marker, so filtering it made
-- the pass blind to its own output and every marker permanent. And the worklist is a UNION,
-- because a company can need a RELEASE without needing a collapse — its second candidate may
-- have closed since. A collapse-only worklist never visits it again, which is how 42 633 open
-- duplicates (prod, 2026-09-01) came to sit behind a closed owner with no pass that would ever
-- let them go. This mirrors CompaniesWithRoleClusters, which unions the same two reasons.
SELECT company_slug
FROM jobs
WHERE closed_at IS NULL AND company_slug <> ''
  AND duplicate_of_aggregator IS NULL AND duplicate_of_role IS NULL
GROUP BY company_slug
HAVING COUNT(*) > 1
UNION
SELECT DISTINCT company_slug
FROM jobs
WHERE closed_at IS NULL AND company_slug <> '' AND duplicate_of_fuzzy IS NOT NULL;

-- name: FuzzyDedupCandidateTitlesForCompany :many
-- The (id, title) of one company's open canonical postings — deliberately WITHOUT the
-- description. The caller groups these into buckets with the same normalized-title function the
-- rest of the codebase uses (jobhash.RoleKey), then loads descriptions for the buckets that
-- survive the size filter via GetJobDescriptionsByIDs. Normalizing here in SQL instead would
-- duplicate that logic in a second language and let the two drift apart; titles are cheap to
-- ship, descriptions are not.
--
-- "Canonical" here means NOT CLAIMED BY AN EXACT PASS, which is why the predicate names their
-- two owned columns rather than the derived duplicate_of. A row carrying this pass's OWN marker
-- is offered again on purpose: it is the only way the pass can ever re-decide it, and without
-- that a marker survives its canon closing, the descriptions diverging, and every later run.
SELECT id, title
FROM jobs
WHERE company_slug = sqlc.arg(company) AND closed_at IS NULL
  AND duplicate_of_aggregator IS NULL AND duplicate_of_role IS NULL
ORDER BY id;

-- name: MarkFuzzyDuplicatesForCompany :one
-- Write one company's whole fuzzy verdict in a single statement rather than a round trip per
-- row: `candidates` is every row the pass CONSIDERED, and (ids, canons) are the ones it
-- clustered. A candidate with no assignment gets NULL — that is the release, and it is the only
-- mechanism there is.
--
-- It has to be, and the comment this replaces said otherwise for a year. Migrations 0114/0115
-- moved the marker into duplicate_of_fuzzy and left duplicate_of to a trigger deriving it from
-- the three owned columns, so the role recompute — which writes duplicate_of_role — can no
-- longer reach this one, and the COALESCE keeps surfacing it. Nothing else releases a fuzzy
-- marker. Measured on prod 2026-09-01: 42 633 open duplicates behind a closed owner, invisible
-- in search, with no pass that would ever let them go.
--
-- `candidates` is deliberately NOT "every open row of the company". It is what the pass looked
-- at, which excludes the buckets it refuses to judge (over the size cap). A cap is a cost
-- heuristic, not a verdict, so releasing what it skipped would silently un-collapse the largest
-- groups in the catalogue. A row in a bucket too SMALL to cluster is a different thing — the
-- pass did judge it, and the answer was "no cluster".
--
-- Scoped to the company and to open rows, so a row the exact pass claimed in the meantime is
-- left alone. The IS DISTINCT FROM guard makes a re-run free.
WITH candidate AS (
    SELECT unnest(sqlc.arg(candidates)::bigint[]) AS id
),
assign AS (
    SELECT unnest(sqlc.arg(ids)::bigint[]) AS id,
           unnest(sqlc.arg(canons)::bigint[]) AS canon_id
),
target AS (
    -- LEFT JOIN is what turns "considered but not clustered" into NULL. Mirrors the CASE in
    -- RecomputeRoleDuplicatesForCompanies, which is how the role pass releases its own.
    SELECT c.id, a.canon_id AS new_dup
    FROM candidate c
    LEFT JOIN assign a ON a.id = c.id
),
before AS (
    -- Every CTE of one statement reads the same snapshot, so this is the derived marker as it
    -- stood BEFORE the update below — which is what makes the status transition readable
    -- without a second statement and the race between them.
    SELECT j.id, j.duplicate_of AS was_duplicate_of
    FROM jobs j JOIN target t ON t.id = j.id
),
updated AS (
    UPDATE jobs j
    SET duplicate_of_fuzzy = m.new_dup,
        updated_at         = now()
    FROM target m
    WHERE j.id = m.id
      AND j.company_slug = sqlc.arg(company)
      AND j.closed_at IS NULL
      AND j.duplicate_of_fuzzy IS DISTINCT FROM m.new_dup
    RETURNING j.id,
              j.duplicate_of AS now_duplicate_of,
              COALESCE(j.posted_at, j.created_at) AS eff_posted_at
),
flipped AS (
    SELECT u.id, b.was_duplicate_of, u.now_duplicate_of, u.eff_posted_at
    FROM updated u JOIN before b ON b.id = u.id
),
dequeued AS (
    INSERT INTO search_delete_outbox (job_id)
    SELECT id FROM flipped WHERE was_duplicate_of IS NULL AND now_duplicate_of IS NOT NULL
    ON CONFLICT (job_id) DO NOTHING
),
requeued AS (
    INSERT INTO search_outbox (job_id, job_posted_at)
    SELECT id, eff_posted_at FROM flipped WHERE was_duplicate_of IS NOT NULL AND now_duplicate_of IS NULL
    ON CONFLICT (job_id) DO NOTHING
)
SELECT count(*)::bigint FROM flipped;

-- name: OrphanAggregatorCompanies :many
-- Companies the catalogue holds ONLY through aggregators — the worklist cmd/harvest-orphans
-- turns into candidate ATS boards. A company qualifies when it has an open posting from one
-- of the REQUESTED aggregators and no open posting from any source outside the FULL
-- aggregator set.
--
-- The two provider sets are deliberately separate. Narrowing a run to one aggregator must
-- not make another aggregator's posting look like first-party ATS coverage: the candidate
-- scan uses `requested`, the exclusion test always uses `aggregators`. Auditing this same
-- distinction with a partial list is what inflated the July aggregator-dedup leak count
-- roughly fourfold.
--
-- The display name is the modal `company` across the aggregator rows, since two aggregators
-- may spell the same employer differently and the name is what the harvest gate compares.
SELECT j.company_slug,
       (mode() WITHIN GROUP (ORDER BY j.company))::text AS company
FROM jobs j
WHERE j.closed_at IS NULL
  AND j.company_slug <> ''
  AND j.source = ANY(sqlc.arg(requested)::text[])
  AND NOT EXISTS (
    SELECT 1 FROM jobs ats
    WHERE ats.company_slug = j.company_slug
      AND ats.closed_at IS NULL
      AND ats.source <> ALL(sqlc.arg(aggregators)::text[])
  )
GROUP BY j.company_slug
ORDER BY count(*) DESC, j.company_slug;

-- name: BackfillCompanySlugFoldedChunk :execrows
-- Fill jobs.company_slug_folded for one id range. The column is maintained by every
-- write path (see migrations/0109), but the rows that predate it need this pass.
--
-- Ranged by id rather than keyset-cursored on the column itself: the whole point is
-- that the column is not yet indexed while this runs, so a WHERE on it would be a seq
-- scan per chunk. The primary key is what makes each chunk a bounded, cheap slice, and
-- it lets a resumed run pick up at a known number instead of a NULL frontier.
--
-- The IS DISTINCT FROM guard makes re-runs free: a chunk whose rows are already correct
-- writes nothing, so no dead tuples and no bloat for repeating the pass. That matters
-- on a 7.4M-row table where an unguarded UPDATE would rewrite every row it touches.
UPDATE jobs
SET company_slug_folded = replace(company_slug, '-', '')
WHERE id >= sqlc.arg(from_id) AND id < sqlc.arg(to_id)
  AND company_slug_folded IS DISTINCT FROM replace(company_slug, '-', '');

-- name: CompanySlugFoldedBackfillBounds :one
-- The id range the backfill walks, plus how many rows still need it. The remaining
-- count is what makes a run's progress legible; it is an exact count on purpose (the
-- pass is run by hand, rarely, and a wrong "0 remaining" would end it early).
SELECT COALESCE(min(id), 0)::bigint AS min_id,
       COALESCE(max(id), 0)::bigint AS max_id,
       count(*) FILTER (WHERE company_slug_folded IS DISTINCT FROM replace(company_slug, '-', ''))::bigint AS remaining
FROM jobs;

-- name: BackfillDuplicateMarkerOwnerChunk :execrows
-- Seed the owned marker columns (0114) from the single duplicate_of that predates them, for one
-- id range.
--
-- Provenance cannot be recovered: a marked row records WHERE it points, never which pass decided
-- it. So the seed goes by shape — a marked row whose own source is an aggregator and whose canon's
-- is not is the aggregator pass's; everything else is seeded as the role pass's.
--
-- Fuzzy markers are indistinguishable from role markers that way and land in the role column. That
-- is self-correcting and cheap to reason about: the first role recompute clears the ones that are
-- not role clusters, and the fuzzy pass re-sets them in its own column during the same run. One
-- extra cycle of the churn this change removes, once.
--
-- Closed rows are seeded too, deliberately. The passes only consider open rows, so nothing would
-- ever seed a closed row's columns — and then the first statement to touch any marker column on it
-- would fire the derivation and silently clear a duplicate_of that prune still walks.
--
-- The "no owned column set yet" predicate is what makes re-runs free, and it is also the reconcile
-- sweep: a row written between this chunk passing its range and the trigger existing still matches,
-- so running the pass again after the trigger lands finishes the job. No separate mode needed.
UPDATE jobs j
SET duplicate_of_aggregator = CASE
        WHEN j.source = ANY(sqlc.arg(aggregators)::text[])
         AND NOT (c.source = ANY(sqlc.arg(aggregators)::text[]))
        THEN j.duplicate_of
    END,
    duplicate_of_role = CASE
        WHEN j.source = ANY(sqlc.arg(aggregators)::text[])
         AND NOT (c.source = ANY(sqlc.arg(aggregators)::text[]))
        THEN NULL
        ELSE j.duplicate_of
    END
FROM jobs c
WHERE c.id = j.duplicate_of
  AND j.id >= sqlc.arg(from_id) AND j.id < sqlc.arg(to_id)
  AND j.duplicate_of IS NOT NULL
  AND j.duplicate_of_aggregator IS NULL
  AND j.duplicate_of_role IS NULL
  AND j.duplicate_of_fuzzy IS NULL;

-- name: DuplicateMarkerOwnerBackfillBounds :one
-- The id range the owned-marker backfill walks, plus how many rows still need it. Exact count on
-- purpose, like the folded-slug bounds beside it: the pass is run by hand and rarely, and a wrong
-- "0 remaining" would end it early.
SELECT COALESCE(min(id), 0)::bigint AS min_id,
       COALESCE(max(id), 0)::bigint AS max_id,
       count(*) FILTER (
           WHERE duplicate_of IS NOT NULL
             AND duplicate_of_aggregator IS NULL
             AND duplicate_of_role IS NULL
             AND duplicate_of_fuzzy IS NULL
       )::bigint AS remaining
FROM jobs;

-- name: JobDescriptionsByIDs :many
-- Descriptions for a named set of ids, for cmd/backfill-clearance.
--
-- Ids come from a Meilisearch query rather than from a SQL predicate on the text, and
-- that is the point: any WHERE over `description` de-TOASTs the column for every row it
-- examines, which on this table means 8M out-of-line reads to find ~38k matches. The
-- search index already holds the text, so it can name the candidates in seconds and
-- this query reads only their bodies.
--
-- Closed rows are included. A closed posting still carries a clearance requirement, the
-- detail endpoint still serves it, and leaving it unmarked would make the facet's
-- meaning depend on lifecycle state.
SELECT id, description FROM jobs
WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: SetJobRequiresClearance :execrows
-- Write one row's requires_clearance, for cmd/backfill-clearance.
--
-- The IS DISTINCT FROM guard is what makes the pass idempotent: a row already carrying
-- the derived value is not rewritten, so a re-run writes nothing, produces no dead
-- tuples, and stopping the pass mid-way costs nothing to resume.
--
-- It also means the backfill never needs to know which rows it has already visited —
-- the guard answers that per row, which is cheaper and more honest than a cursor that
-- would go stale the moment ingest writes a new posting behind it.
UPDATE jobs
SET requires_clearance = sqlc.arg(requires_clearance)
WHERE id = sqlc.arg(id)
  AND requires_clearance IS DISTINCT FROM sqlc.arg(requires_clearance);
