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
FROM jobs_read
WHERE closed_at IS NULL AND duplicate_of IS NULL AND NOT is_private
ORDER BY created_at DESC, id DESC
LIMIT $1 OFFSET $2;

-- name: ListJobsByIDAfter :many
-- Keyset scan for the reindex command: pages by the immutable primary key, so
-- concurrent inserts/updates (which shift posted_at ordering) cannot make the
-- scan skip or repeat rows the way OFFSET pagination would.
SELECT *
FROM jobs_read
WHERE id > sqlc.arg(after_id)
ORDER BY id
LIMIT sqlc.arg(batch_size);

-- name: ListJobsBySourceAfter :many
-- Keyset scan over one provider's rows, for the per-source repair workers (e.g.
-- cmd/backfill-echojobs, cmd/backfill-descriptions): pages by the immutable primary key
-- (concurrent writes can't skip or repeat rows) filtered to a single source. Returns
-- closed rows too — a one-time backfill of a missing description fills open and closed alike.
SELECT *
FROM jobs_read
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
FROM jobs_read
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

-- name: ListLiveJobsByIDAfter :many
-- ListJobsByIDAfter narrowed to the rows that can still reach the catalogue, for a
-- re-derive after a dictionary change (cmd/backfill-derive with
-- BACKFILL_DERIVE_CLOSED_WITHIN_DAYS).
--
-- Measured 2026-09-23: the table holds ~12.7M rows and 1.9M open ones, so the derive
-- pass spends ~85% of its time on postings nothing can surface. A pass over the whole
-- table takes ~30h at the unit's deliberately low CPUWeight.
--
-- NOT simply `closed_at IS NULL`, and this is the load-bearing part. A closed posting
-- REOPENS: ingest's Toucher refreshes liveness "(last_seen_at, reopen if closed) ...
-- WITHOUT rewriting its content" (internal/ingest/pipeline/pipeline_ports.go), and a posting
-- that merely drifts out of a feed for 48h is closed and reopened as it drifts back
-- (see the notes in sources/seek.go and sources/whatjobs.go). Skipping it on
-- `closed_at IS NULL` would return it to the catalogue carrying the facets the old
-- dictionary gave it, with nothing downstream reporting the staleness.
--
-- So the window is "open, or closed recently enough to plausibly come back". The caller
-- picks the cutoff; the pass degrades to the full table when it passes the zero time,
-- which keeps the unfiltered behaviour one env var away.
SELECT *
FROM jobs_read
WHERE id > sqlc.arg(after_id)
  AND (closed_at IS NULL OR closed_at >= sqlc.arg(closed_since))
ORDER BY id
LIMIT sqlc.arg(batch_size);

-- name: ListLiveJobIDsAfter :many
-- Id-only projection of ListLiveJobsByIDAfter, for the same corruption-degrade path
-- ListJobIDsAfter serves. The predicate must match its wide sibling exactly: a
-- degraded re-read that scanned a different window would skip rows silently.
SELECT id
FROM jobs
WHERE id > sqlc.arg(after_id)
  AND (closed_at IS NULL OR closed_at >= sqlc.arg(closed_since))
ORDER BY id
LIMIT sqlc.arg(batch_size);

-- name: ListIndexableJobsByIDAfter :many
-- The full rebuild's keyset scan (cmd/reindex), narrowed in SQL to the rows a search
-- document can come from: open, canonical, public. That is the cheap half of
-- cmd/reindex's splitJobs test; the half SQL cannot express (the category and the
-- visible-description rules) stays in Go.
--
-- The rebuild used to page ListJobsByIDAfter — every row ever stored — and drop what
-- splitJobs rejected. Nothing deletes closed rows any more, so that scan grew with
-- history rather than with the catalogue. Measured on prod 2026-10-22: 18.8M rows
-- read to index 3.78M documents, ~50k rows/min regardless of how many survived, so
-- the streaming phase took 6.4h (~21% of the oldest id decile is open and canonical,
-- 64% of the newest — the per-minute document rate tracked exactly that). This
-- predicate is jobs_sitemap_idx's own, so the scan walks that partial index (~6.2M
-- entries) instead of the primary key.
SELECT *
FROM jobs_read
WHERE id > sqlc.arg(after_id)
  AND closed_at IS NULL AND duplicate_of IS NULL AND NOT is_private
ORDER BY id
LIMIT sqlc.arg(batch_size);

-- name: ListIndexableJobIDsAfter :many
-- Id-only projection of ListIndexableJobsByIDAfter, for the corruption-degrade path
-- ListJobIDsAfter serves. The predicate must match its wide sibling exactly: a
-- degraded re-read that scanned a different window would skip rows silently.
SELECT id
FROM jobs
WHERE id > sqlc.arg(after_id)
  AND closed_at IS NULL AND duplicate_of IS NULL AND NOT is_private
ORDER BY id
LIMIT sqlc.arg(batch_size);

-- name: GetJob :one
SELECT *
FROM jobs_read
WHERE id = $1;

-- name: GetJobBySlug :one
SELECT *
FROM jobs_read
WHERE public_slug = $1;

-- name: ListJobsBySlugCode :many
-- The jobs whose slug ends with a shortcode — the last eight characters of a public_slug,
-- derived from (source, external_id) alone and so the one part of a job URL nothing can
-- move. Resolves a link to a slug that no longer exists (freehire#3284); backed by
-- jobs_public_slug_code_idx. The caller distinguishes full-hash collision recovery
-- URLs before refusing ambiguous ordinary shortcodes.
SELECT *
FROM jobs_read
WHERE right(public_slug, 8) = sqlc.arg(code)::text;

-- name: ListJobLivenessBySlugs :many
-- What the ChatGPT app's check_jobs_open needs to say whether each posting in a shortlist is
-- still up: the visibility columns its predicate reads plus the two liveness stamps, and
-- nothing else. One round trip for the whole list; an unknown slug is simply absent, which the
-- caller reports as not found.
SELECT public_slug, is_private, duplicate_of, closed_at, last_seen_at
FROM jobs_read
WHERE public_slug = ANY(sqlc.arg('slugs')::text[]);

-- name: StoredJobDescription :one
-- The body a stored posting keeps. Ingest reads it for a posting whose detail fetch came back
-- empty, so the facets it derives are those of the body UpsertJob preserves, not of nothing.
SELECT description FROM jobs WHERE source = sqlc.arg(source) AND external_id = sqlc.arg(external_id);

-- name: GetJobBySourceExternalID :one
-- Load a job by its dedup identity (source, external_id) — the key the Job
-- aggregate's repository loads by, mirroring the CloseJobBySourceExternalID key.
SELECT *
FROM jobs_read
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
--
-- A link that normalizes to nothing matches nothing: a posting with no outside page
-- stores an empty url, and "https://www." must not answer with one of those.
SELECT COALESCE(canonical.public_slug, j.public_slug)
FROM jobs j
LEFT JOIN jobs canonical ON canonical.id = j.duplicate_of AND canonical.closed_at IS NULL
LEFT JOIN job_activity a ON a.job_id = j.id
WHERE normalize_job_url(j.url) = normalize_job_url(@url)
  AND normalize_job_url(@url) <> ''
  AND j.closed_at IS NULL
ORDER BY (j.duplicate_of IS NULL) DESC, a.last_seen_at DESC, j.id DESC
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
--
-- AND NOT is_private excludes the jd-tailor-intake private-job path. InsertPrivateJob
-- derives a company_slug from whatever employer the pasted text names, so without this a
-- private JD naming a company the catalogue already knows appears in that company's PUBLIC
-- list — full pasted text, and first, since this orders by created_at DESC. The route is
-- anonymous (GET /api/v1/companies/:slug/jobs, mounted under mw.optional), so the leak
-- needs no account at all, and the same query backs the assistant's company-jobs tool.
-- RefreshCompanyFacets already excludes private rows from companies.job_count, so before
-- this clause the page contradicted its own counter.
SELECT *
FROM jobs_read
WHERE company_slug = $1 AND closed_at IS NULL AND duplicate_of IS NULL AND NOT is_private
ORDER BY created_at DESC, id DESC
LIMIT $2 OFFSET $3;

-- name: ListClosedJobsByCompany :many
-- The company page's Closed tab: the postings this company has closed, most recently closed
-- first. The same exclusions as ListJobsByCompany (reposts, private pasted JDs) with the
-- closed_at predicate inverted.
--
-- Served by jobs_closed_company_closed_at_idx (migration 0194): (company_slug, closed_at DESC,
-- id DESC), PARTIAL on exactly this WHERE. Partial because the broader jobs_company_slug_idx
-- leaves closed_at/duplicate_of/is_private as heap filters, so page 1 walked every open
-- posting first (dominos, 14.5k open rows: 5.4s cold). The ORDER BY must match the index
-- exactly: a large employer holds hundreds of thousands of closed rows, and any other order
-- is a sort over all of them. That is also why nothing counts this set — see the handler.
SELECT *
FROM jobs_read
WHERE company_slug = $1 AND closed_at IS NOT NULL AND duplicate_of IS NULL AND NOT is_private
ORDER BY closed_at DESC, id DESC
LIMIT $2 OFFSET $3;

-- name: MaxJobID :one
-- The top of the id space cmd/serve-probe draws its random sample windows from. One
-- backward probe of the primary key, so it stays cheap on the whole table.
SELECT COALESCE(MAX(id), 0)::bigint FROM jobs;

-- name: ListPendingSearchRemovals :many
-- Which of these jobs are closed but still have a document in the live index: the ones
-- search_delete_outbox has queued and the drain has not yet applied. A search page reads
-- its hits from Meilisearch alone, so without this a posting stays in the results for as long
-- as the drain is paused (a facet rebuild holds it for hours). The primary-key lookup is
-- bounded by the page size.
-- A queued row is not enough on its own: TouchJob and UpsertJob reopen a posting without
-- dequeuing it, and a reopened posting is open and must stay in the results. So the job
-- must be gone or still closed. A missing row stays hidden (the removal outlives the job,
-- see the queue's missing foreign key).
SELECT d.job_id FROM search_delete_outbox d
LEFT JOIN jobs j ON j.id = d.job_id
WHERE d.job_id = ANY(sqlc.arg(job_ids)::bigint[])
  AND (j.id IS NULL OR j.closed_at IS NOT NULL);

