-- name: CloseStaleUnsignalledJobs :one
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
--
-- The removal enqueue rides this statement for the reason CloseUnseenJobs gives, and it was
-- missing here until 2026-09-06: a posting closed by age stayed in the facet index until the
-- next full rebuild, which is how a closed job goes on being served by search.
--
-- :one rather than :execrows because the CTE moves the row count out of the command tag.
-- count(*) over the closed rows is the same int64 the caller already had.
WITH closed AS (
    UPDATE jobs
    SET closed_at     = now(),
        closed_reason = 'expired',
        updated_at    = now()
    WHERE closed_at IS NULL
      AND source = ANY(sqlc.arg(sources)::text[])
      AND COALESCE(posted_at, created_at) < sqlc.arg(cutoff)
    RETURNING id
), queued AS (
    INSERT INTO search_delete_outbox (job_id)
    SELECT id FROM closed
    ON CONFLICT (job_id) DO NOTHING
)
SELECT count(*) FROM closed;

-- name: CloseStaleUnseenUnprobeableJobs :one
-- The same age rule as CloseStaleUnsignalledJobs, for a source that IS re-crawled but
-- whose postings a probe can never judge — whatjobs today, whose stored url is the ad
-- network's own tracking landing page and answers identically whether or not the posting
-- behind it is live (cmd/liveness's expireDespiteRegisteredPrefixes).
--
-- It takes a SECOND clock, and that is the whole point of it being a separate statement.
-- CloseStaleUnsignalledJobs asks only how old a posting is, which is sound for a source
-- nothing re-crawls: last_seen_at there is frozen at ingest, so age is the only fact
-- available. Re-using it for whatjobs closed postings the crawl was still listing twice a
-- day — whatjobs leaves posted_at unset, so the 45-day clock runs from first ingest — and
-- the next crawl reopened each one through UpsertJob. That flap costs a full re-upsert
-- instead of a cheap RefreshUnchangedJob, restamps updated_at (the sitemap's <lastmod>),
-- drops and recomputes the semantic chunks, and mails everyone who applied to it that
-- their posting closed.
--
-- seen_cutoff is what makes the guess honest: only a posting the crawl has ALSO stopped
-- listing for longer than the sweep's own grace window is presumed filled — i.e. one the
-- sweep would already have closed had its company_slug scope been able to reach it. The
-- caller owns both windows, as everywhere else in this family.
--
-- Both predicates fail CLOSED on an empty list and on a NULL cutoff, exactly as the
-- statement above does.
--
-- last_seen_at is job_activity's; locking and freshness recheck: see CloseUnseenJobs.
WITH cand AS (
    SELECT j.id
    FROM jobs j
    JOIN job_activity a ON a.job_id = j.id
    WHERE j.closed_at IS NULL AND NOT j.is_private
      AND j.source = ANY(sqlc.arg(sources)::text[])
      AND COALESCE(j.posted_at, j.created_at) < sqlc.arg(cutoff)
      AND a.last_seen_at < sqlc.arg(seen_cutoff)
    ORDER BY j.id
    FOR NO KEY UPDATE OF j, a
), closed AS (
    UPDATE jobs
    SET closed_at     = now(),
        closed_reason = 'expired',
        updated_at    = now()
    FROM cand
    WHERE jobs.id = cand.id AND jobs.closed_at IS NULL
    RETURNING jobs.id
), queued AS (
    INSERT INTO search_delete_outbox (job_id)
    SELECT id FROM closed
    ON CONFLICT (job_id) DO NOTHING
)
SELECT count(*) FROM closed;

-- name: SelectOrphanLivenessCandidates :many
-- Orphan-job liveness (probe-orphan-job-liveness): open jobs whose source is NOT a
-- registered ATS board provider — the sources no ingest run re-crawls and the sweep
-- therefore never closes (telegram, habr_career, geekjob, …). The caller passes the
-- ATS provider set from the sources registry; <> ALL excludes them, so a new adapter
-- never silently becomes a probe target. Closed jobs are skipped (already not open), and
-- so is a posting with no outside page (an employer vacancy taking applications on
-- freehire): there is nothing to request, and its employer closes it.
SELECT j.id, j.source, j.url, j.public_slug, COALESCE(a.liveness_strikes, 0)::integer AS liveness_strikes
FROM jobs j
LEFT JOIN job_activity a ON a.job_id = j.id
WHERE j.closed_at IS NULL
  AND j.url <> ''
  AND j.source <> ALL(sqlc.arg(ats_providers)::text[]);

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
SELECT j.id, j.source, j.url, j.external_id, j.public_slug, a.liveness_strikes
FROM jobs j
JOIN job_activity a ON a.job_id = j.id
WHERE j.closed_at IS NULL
  AND j.source = ANY(sqlc.arg(sources)::text[])
  AND a.last_seen_at < sqlc.arg(cutoff);

-- name: SelectScraplingLivenessCandidates :many
-- cmd/adzuna-liveness's candidates: open postings of one source the crawl has stopped listing.
-- For adzuna "unseen" is not evidence of removal (its feed is far deeper than the crawl's
-- request budget: 157k US it-jobs postings in 14 days, 750 read per run); for wellfound it
-- strands a posting whose company left every role slice the crawl reads, out of the sweep's
-- reach. The source's own page settles both, and that worker reads it.
--
-- ORDER BY random() rather than by age because most adzuna probes end on a challenge page and
-- leave the row untouched: a fixed order would hand the same unanswered rows to every
-- bounded run and never reach the rest. Sampling reaches every row with nothing to store.
SELECT j.id, j.external_id, j.url, j.public_slug, a.liveness_strikes
FROM jobs j
JOIN job_activity a ON a.job_id = j.id
WHERE j.closed_at IS NULL
  AND NOT j.is_private
  AND j.source = sqlc.arg(source)
  AND a.last_seen_at < sqlc.arg(cutoff)
ORDER BY random()
LIMIT sqlc.arg(max_rows);

-- name: MarkLivenessExpired :one
-- Record one expired probe: increment the strike counter and, in the same write,
-- close the job (closed_at) once it reaches the threshold the caller owns — the
-- two-strike grace that absorbs a transient death signal. Returns the new strike
-- count and closed_at so the worker can log the outcome.
--
-- The removal enqueue rides this statement like every other close (see CloseUnseenJobs),
-- gated on the close actually happening: most calls only advance a strike and must queue
-- nothing. The gate reads the rows AS LOCKED below, so it is exact.
--
-- seen_before, when set, is the staleness cutoff the caller selected its candidates by. A
-- probe answers minutes after that read, and a crawl that listed the posting again in between
-- has just proved it live; the verdict was reached on a stale read and must write nothing — no
-- strike, no close, no queued removal — so the statement matches no row (pgx.ErrNoRows). NULL
-- keeps the old behaviour for a caller whose candidates no crawl re-lists.
--
-- Locking and the compare-and-set: see CloseUnseenJobs. `locked` reads the strike count from the
-- LOCKED activity row, the one value every writer keeps current.
--
-- Release A writes the count to BOTH places: job_activity explicitly, and the legacy jobs column
-- for readers older than release A. The one-way bridge fires only when the legacy value CHANGES,
-- and while writers move to job_activity the legacy value goes stale, so a computed count that
-- happens to equal the stale legacy one would never reach job_activity through it. Both writes
-- carry the same value, so the bridge firing as well is idempotent.
WITH locked AS MATERIALIZED (
    SELECT j.id, j.closed_at, a.liveness_strikes
    FROM jobs j
    JOIN job_activity a ON a.job_id = j.id
    WHERE j.id = sqlc.arg(id)
      AND (sqlc.narg(seen_before)::timestamptz IS NULL OR a.last_seen_at < sqlc.narg(seen_before))
    FOR NO KEY UPDATE OF j, a
), bumped AS (
    UPDATE job_activity a
    SET liveness_strikes = l.liveness_strikes + 1
    FROM locked l
    WHERE a.job_id = l.id
    RETURNING a.job_id, a.liveness_strikes AS strikes,
              (l.closed_at IS NULL AND a.liveness_strikes >= sqlc.arg(threshold)) AS closing
), queued AS (
    INSERT INTO search_delete_outbox (job_id)
    SELECT b.job_id FROM bumped b WHERE b.closing
    ON CONFLICT (job_id) DO NOTHING
)
-- FROM bumped, not FROM locked: the data dependency makes the activity write happen before the jobs
-- write (and before the triggers that jobs write fires), instead of relying on the order Postgres
-- happens to run independent data-modifying CTEs in.
UPDATE jobs
SET liveness_strikes = b.strikes,
    -- A close only when `locked` saw the row open: if another mechanism closed it between candidate
    -- selection and this write, its closed_at/closed_reason are the true record of when and why, and
    -- this probe must not overwrite either.
    closed_at = CASE WHEN b.closing THEN now() ELSE jobs.closed_at END,
    closed_reason = CASE WHEN b.closing THEN 'probe_expired' ELSE jobs.closed_reason END,
    updated_at = now()
FROM bumped b
WHERE jobs.id = b.job_id
RETURNING jobs.id, jobs.liveness_strikes, jobs.closed_at;

-- name: ResetLivenessStrikes :exec
-- A healthy (not-expired) probe clears any accumulated strikes, so only CONSECUTIVE
-- expired probes can close a job. Guarded to the non-zero case so probing an
-- already-clean job does not churn the row or lock it.
--
-- The guard reads job_activity, the current count, and both places are written (see
-- MarkLivenessExpired for why): a legacy column that is already 0 while job_activity holds 4 must
-- still reset job_activity, and the bridge would not fire for a legacy 0 -> 0. The legacy column is
-- written only when it is not already 0, so the wide jobs tuple is untouched in that case. Locking:
-- see CloseUnseenJobs.
WITH locked AS MATERIALIZED (
    SELECT j.id
    FROM jobs j
    JOIN job_activity a ON a.job_id = j.id
    WHERE j.id = sqlc.arg(id) AND a.liveness_strikes <> 0
    FOR NO KEY UPDATE OF j, a
), cleared AS (
    UPDATE job_activity a
    SET liveness_strikes = 0
    FROM locked l
    WHERE a.job_id = l.id
    RETURNING a.job_id
)
UPDATE jobs
SET liveness_strikes = 0
FROM cleared c
WHERE jobs.id = c.job_id AND jobs.liveness_strikes <> 0;
