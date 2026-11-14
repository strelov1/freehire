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
--
-- A row whose stored BODY has gone stale is withheld too, so the crawl re-reads a posting the
-- employer has since edited — the one way an edit reaches a catalogue where being stored is
-- what stops a posting from ever being fetched again. Stale means hydrated_at older than
-- body_refresh_cutoff, NULL (never checked) included. body_refresh_slot bounds the cost: a
-- posting belongs to one slot for its life (hashtext is deterministic within a database), so a
-- run re-reads roughly 1/body_refresh_slices of the stale rows instead of facing a hydrating
-- provider's whole catalogue at once — 1.27M postings on workday. A slot of -1 never matches
-- any row, which is how the caller expresses "disabled" without a second query.
--
-- hashtext is cast to bigint before abs(): abs() of int4's most negative value raises.
--
-- A row whose stored application deadline has passed, but no earlier than
-- deadline_recheck_from, is withheld as well. The listing carries no deadline (EURES states it
-- only on the detail), so a re-listed posting's touch would close it on the stored date even
-- when the employer has since extended it; re-reading the detail lets the upsert store the
-- current date and reopen the row. Bounded, because a source that keeps listing an expired
-- posting would otherwise cost a detail request on every crawl for good. Written as an OR over
-- the kept cases rather than NOT over the withheld one: a NULL deadline (every source but EURES)
-- would make the NOT form NULL and drop the row from the set.
--
-- hydrated_at is job_activity's. LEFT JOIN, so a job without an activity row reads as never
-- checked (NULL, the same "stale" answer a legacy NULL gave) instead of vanishing from the set.
SELECT j.external_id, j.is_tech FROM jobs j
LEFT JOIN job_activity a ON a.job_id = j.id
WHERE j.source = sqlc.arg(source)
  AND (j.description <> '' OR j.created_at < sqlc.arg(hydration_cutoff))
  AND (a.hydrated_at >= sqlc.arg(body_refresh_cutoff)
       OR abs(hashtext(j.external_id)::bigint) % sqlc.arg(body_refresh_slices)::bigint
          <> sqlc.arg(body_refresh_slot)::bigint)
  AND (j.application_deadline IS NULL
       OR j.application_deadline >= (now() AT TIME ZONE 'UTC')::date
       OR j.application_deadline < sqlc.arg(deadline_recheck_from)::date);

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
-- hydration_cutoff withholds a still-body-less row from the seen-set so its detail is retried,
-- and the body_refresh_* arm withholds one whose stored body has gone stale so an employer's
-- edit is re-read, and the deadline arm re-reads a posting whose stored deadline has just
-- passed; see ExistingExternalIDs for all three.
--
-- hydrated_at is job_activity's, read through a LEFT JOIN as in ExistingExternalIDs.
SELECT j.external_id, j.is_tech FROM jobs j
LEFT JOIN job_activity a ON a.job_id = j.id
WHERE j.source = sqlc.arg(source)
  AND j.external_id LIKE sqlc.arg(pattern)
  AND (j.description <> '' OR j.created_at < sqlc.arg(hydration_cutoff))
  AND (a.hydrated_at >= sqlc.arg(body_refresh_cutoff)
       OR abs(hashtext(j.external_id)::bigint) % sqlc.arg(body_refresh_slices)::bigint
          <> sqlc.arg(body_refresh_slot)::bigint)
  AND (j.application_deadline IS NULL
       OR j.application_deadline >= (now() AT TIME ZONE 'UTC')::date
       OR j.application_deadline < sqlc.arg(deadline_recheck_from)::date);

-- name: TouchJob :one
-- Liveness refresh for a hydrating source's already-ingested posting (see source-ingest): the
-- crawl re-listed the offer but fetched no fresh content (detail is fetched only for new
-- offers), so refresh last_seen_at and reopen if it had been closed, unless its stored
-- application deadline has passed. Expiry closes and queues removal atomically, WITHOUT touching the
-- content columns. A full upsert of the content-less listing would re-derive the deterministic
-- facets from an empty description and wipe the row's hydrated description/skills. This is the
-- reopen half of UpsertJob's ON CONFLICT, minus every content write. A closed-to-open
-- transition queues search restoration atomically; open-to-open touches queue nothing.
-- RETURNING company_slug so
-- the caller records the company into the crawled-set that scopes the post-run unseen sweep —
-- exactly as UpsertJob's write path does — otherwise a company whose offers were all touched
-- (not newly saved) would drop out of the sweep and its removed offers would never close.
-- Lock the prior state before updating: after waiting for a concurrent close,
-- transition detection must observe the committed closed row, not an older snapshot.
-- FOR NO KEY UPDATE, not FOR UPDATE: a foreign-key child insert (enrichment_outbox, apply_forms,
-- search_outbox) holds KEY SHARE on this tuple while a toucher already holds other locks of its
-- own, and FOR UPDATE conflicts with KEY SHARE (a deadlock waiting to happen); a plain UPDATE's
-- lock does not, and it is all this statement needs. The jobs tuple AND the activity row are locked
-- here, in that order, before anything is queued: release A still writes the legacy columns and its
-- one-way bridge would take the activity row only at the end of the statement, AFTER the
-- search_outbox writes below, while a writer that locks the activity row first and then queues
-- (release B) deadlocks with that. last_seen_at, the previous stamp the withheld-then-seen requeue
-- compares, is the locked activity row's.
WITH existing AS MATERIALIZED (
    SELECT j.id, j.closed_at, j.is_private, j.duplicate_of, a.last_seen_at, j.application_deadline AS previous_deadline,
           COALESCE(sqlc.narg(application_deadline)::date, j.application_deadline) AS deadline,
           -- The touch would reopen this row: closed, and its deadline (if any) not passed.
           (j.closed_at IS NOT NULL AND (COALESCE(sqlc.narg(application_deadline)::date, j.application_deadline) IS NULL
                                         OR COALESCE(sqlc.narg(application_deadline)::date, j.application_deadline) >= (now() AT TIME ZONE 'UTC')::date)) AS would_reopen
    FROM jobs j
    JOIN job_activity a ON a.job_id = j.id
    WHERE j.source = sqlc.arg(source) AND j.external_id = sqlc.arg(external_id)
    FOR NO KEY UPDATE OF j, a
), touched AS (
    UPDATE jobs
    SET last_seen_at = now(),
        application_deadline = e.deadline,
        closed_at = CASE WHEN e.deadline < (now() AT TIME ZONE 'UTC')::date
                         THEN COALESCE(jobs.closed_at, now()) ELSE NULL END,
        closed_reason = CASE WHEN e.deadline < (now() AT TIME ZONE 'UTC')::date
                             THEN 'expired' ELSE '' END,
        liveness_strikes = CASE WHEN jobs.closed_at IS NOT NULL THEN 0 ELSE liveness_strikes END,
        -- Stamp a lifecycle transition, not another observation of an expired posting.
        updated_at = CASE
            WHEN (jobs.closed_at IS NULL) IS DISTINCT FROM
                 (e.deadline IS NULL OR e.deadline >= (now() AT TIME ZONE 'UTC')::date)
            OR jobs.application_deadline IS DISTINCT FROM e.deadline
            THEN now() ELSE updated_at END
    FROM existing e
    WHERE jobs.id = e.id AND (sqlc.arg(allow_reopen)::boolean OR NOT e.would_reopen)
    RETURNING jobs.id, jobs.company_slug, jobs.closed_at,
              jobs.application_deadline, COALESCE(jobs.posted_at, jobs.created_at) AS job_posted_at
), queued AS (
    INSERT INTO search_delete_outbox (job_id)
    SELECT t.id FROM touched t JOIN existing e ON e.id = t.id
    WHERE t.closed_at IS NOT NULL AND e.closed_at IS NULL AND NOT e.is_private
    ON CONFLICT (job_id) DO NOTHING
), changed_queued AS (
    -- Reopen, changed listing metadata, or re-seen after search withheld it.
    INSERT INTO search_outbox (job_id, job_posted_at)
    SELECT t.id, t.job_posted_at
    FROM touched t JOIN existing e ON e.id = t.id
    WHERE t.closed_at IS NULL AND NOT e.is_private
      AND (e.closed_at IS NOT NULL
           OR t.application_deadline IS DISTINCT FROM e.previous_deadline
           OR (e.duplicate_of IS NULL
               AND e.last_seen_at < sqlc.arg(withheld_if_seen_before)::timestamptz))
    ON CONFLICT (job_id) DO UPDATE
        SET id = DEFAULT, created_at = now(), attempts = 0, last_error = '', failed_at = NULL
)
-- A touch writes no content, so a posting closed under an older dictionary would reopen
-- carrying it. With allow_reopen false a touch that WOULD reopen writes nothing and reports
-- deferred; the caller repeats it with allow_reopen true inside the transaction that re-derives
-- the row (internal/ingest/rederive), so a failed re-derive leaves the row closed for the next
-- crawl to retry instead of open on stale facets. reopened is the transition this call made.
SELECT e.id, COALESCE(t.company_slug, '')::text AS company_slug,
       (t.id IS NULL)::boolean AS deferred,
       (t.id IS NOT NULL AND t.closed_at IS NULL AND e.closed_at IS NOT NULL)::boolean AS reopened
FROM existing e LEFT JOIN touched t ON t.id = e.id;

-- name: CloseJobsPastApplicationDeadline :one
-- The source's deadline is inclusive: it expires on the following UTC date. Independent of
-- crawl success/quiet periods, and atomically queued for removal from the live search index.
WITH closed AS (
    UPDATE jobs
    SET closed_at = now(), closed_reason = 'expired', updated_at = now()
    WHERE application_deadline < (now() AT TIME ZONE 'UTC')::date
      AND closed_at IS NULL AND NOT is_private
      AND (sqlc.arg(source)::text = '' OR jobs.source = sqlc.arg(source)::text)
    RETURNING id
), queued AS (
    INSERT INTO search_delete_outbox (job_id)
    SELECT id FROM closed
    ON CONFLICT (job_id) DO NOTHING
)
SELECT count(*) FROM closed;

-- name: RefreshCoveredJobs :many
-- Liveness refresh for the postings an aggregator board listed but the coverage gate kept out
-- of the write path (pipeline.CoveredRefresher): the employer is already crawled first-hand, so
-- nothing is written — but a row stored before that began must not be left with the
-- last_seen_at of its last write, or it stays open whether or not the aggregator still lists it
-- (4dayweek, 2026-10-30: 17,306 of 23,046 open rows unseen for a week, 98% of them covered).
-- last_seen_at is the ONLY column written, for RefreshUnchangedJob's reason: it is in no index,
-- so the update is heap-only. Open rows only, and never a reopen: reopening is a write the gate
-- has already declined. An id the catalogue does not hold matches nothing. RETURNING
-- company_slug so the caller puts the company in the sweep's crawled scope, as TouchJob does.
UPDATE jobs
SET last_seen_at = now()
WHERE source = sqlc.arg(source)
  AND external_id = ANY(sqlc.arg(external_ids)::text[])
  AND closed_at IS NULL
  AND NOT is_private
RETURNING company_slug;

-- name: CloseCoveredJobs :many
-- Close the open stored rows an aggregator board listed for an employer the coverage gate
-- found already crawled first-hand (pipeline.CoveredCloser). Until 2026-11-08 these rows were
-- only refreshed (RefreshCoveredJobs), so a copy stored before the employer's own board joined
-- the fleet stayed visible beside it whenever its title differed from every ATS posting —
-- 4,784 open whatjobs/adzuna rows under 1,354 covered companies when measured. Closing is
-- reversible: a lapsed coverage lets the next crawl write the posting, and UpsertJob reopens
-- it. The removal enqueue rides the statement, as in CloseUnseenJobs. RETURNING company_slug
-- so the caller records the company as crawled, as RefreshCovered does — and for a listed row
-- an earlier crawl already closed this way too, or a provider whose every posting is covered
-- would show no crawl evidence from its second run on and never sweep its delisted rows.
WITH already AS (
    SELECT j.company_slug FROM jobs j
    WHERE j.source = sqlc.arg(source)
      AND j.external_id = ANY(sqlc.arg(external_ids)::text[])
      AND j.closed_reason = 'ats_covered'
      AND j.closed_at IS NOT NULL
      AND NOT j.is_private
), closed AS (
    UPDATE jobs
    SET closed_at     = now(),
        closed_reason = 'ats_covered',
        updated_at    = now()
    WHERE source = sqlc.arg(source)
      AND external_id = ANY(sqlc.arg(external_ids)::text[])
      AND closed_at IS NULL
    RETURNING id, company_slug
), queued AS (
    INSERT INTO search_delete_outbox (job_id)
    SELECT id FROM closed
    ON CONFLICT (job_id) DO NOTHING
)
SELECT company_slug FROM closed
UNION ALL
SELECT company_slug FROM already;

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
