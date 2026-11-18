-- name: CloseUnseenJobs :one
-- skipped_board_patterns (externalid.BoardPattern of every board the run SKIPPED — in quiet pause
-- or failure cooldown) are excluded: a skipped board was not looked at, so its rows' age since
-- last seen says nothing about them, and a quiet pause outlasts the 48h window. COALESCE because a
-- NULL array would make NOT (LIKE ANY(NULL)) NULL and silently close nothing at all.
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
-- Observation state lives in job_activity (release A of the job_activity split), so the candidates
-- are chosen by a locking SELECT over jobs JOIN job_activity: FOR NO KEY UPDATE OF j, a locks the
-- jobs tuple first and the activity row second (the one global lock order, which the bridge trigger
-- and every writer share) and RE-EVALUATES `a.last_seen_at < cutoff` on the LATEST version of each
-- row after waiting for a concurrent writer. A plain UPDATE jobs ... FROM job_activity would not: the
-- activity row is not the UPDATE's target, so Postgres would keep the snapshot version and close a
-- posting a crawl had just freshened. NO KEY UPDATE, never FOR UPDATE: a foreign-key child insert
-- (enrichment_outbox, apply_forms, search_outbox) holds KEY SHARE on the jobs tuple while a cheap
-- toucher already holds the activity row, and FOR UPDATE would deadlock against it. ORDER BY j.id
-- takes the locks in one order across overlapping sweeps. INNER JOIN: the lock needs the row, and
-- every job has one (insert trigger + backfill gate).
WITH cand AS (
    SELECT j.id
    FROM jobs j
    JOIN job_activity a ON a.job_id = j.id
    WHERE j.closed_at IS NULL AND NOT j.is_private
      AND j.source = sqlc.arg(source)
      AND a.last_seen_at < sqlc.arg(cutoff)
      AND j.company_slug = ANY(sqlc.arg(company_slugs)::text[])
      AND NOT (j.external_id LIKE ANY(COALESCE(sqlc.arg(skipped_board_patterns)::text[], '{}')))
    ORDER BY j.id
    FOR NO KEY UPDATE OF j, a
), closed AS (
    UPDATE jobs
    SET closed_at     = now(),
        closed_reason = 'unseen',
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

-- name: CloseUnseenJobsForBoard :one
-- Board-scoped post-ingest sweep (see job-lifecycle spec, freehire#2328): close every open
-- job of ONE board not seen since the cutoff, regardless of which company_slug the run wrote
-- a job for. This is what CloseUnseenJobs' company scope structurally cannot reach: a company
-- whose LAST posting drops off a board the fleet still crawls never re-enters the crawled-slug
-- set, so its row stays open forever under the company scope alone. The board scope answers a
-- narrower, board-level question instead — "did this run prove it listed this board's content"
-- — which is answerable even when a company wrote nothing this run.
--
-- cmd/ingest gates this to a board whose run proved coverage (crawl did not fail, the board
-- yielded at least one posting) AND whose provider is registered as fullBoardListing — an
-- adapter that structurally proves it lists a board to completion rather than silently
-- truncating it. Without that gate this statement is unsafe: an adapter that returns a
-- partial listing as an unqualified success (the solidjobs shape that forced #2337's revert)
-- would have this close everything past the point it stopped reaching.
--
-- board_pattern is externalid.BoardPattern(board) — the same escaped LIKE prefix
-- ExistingExternalIDsByBoard and BackfillBoardCompany already use, riding the
-- (source, external_id text_pattern_ops) index so this reads one board's rows, not the
-- provider's whole catalogue. The ":" namespace terminator inside the pattern is what keeps a
-- board whose id is a prefix of another's (e.g. "it" vs "it2") from also matching the other's
-- rows.
--
-- The search_delete_outbox CTE is copied verbatim from CloseUnseenJobs: the enqueue must ride
-- this statement so it stays atomic with the close (a rolled-back sweep queues nothing) and
-- exact (only rows that actually closed are queued) — without it a board-scoped close would
-- retire rows in Postgres and leave every one of them in the search index until the next full
-- rebuild.
--
-- closed_reason stays 'unseen', not a new value: this is the same mechanism — a crawl of the
-- place this posting lived did not list it — reaching rows its scope previously could not.
--
-- url_pattern narrows the close to one REGION of a board, for an adapter whose board id repeats
-- across regions while external_id carries no region (jobstreet: subclass "6290" is a board in
-- six markets). Each region's postings live on that market's own host, so the pattern is the
-- LIKE-escaped prefix of the region's posting URLs (sources.BoardURLPrefix); '' is no narrowing,
-- which is every other provider. A predicate on the url rather than a region in external_id
-- because the latter would re-key every stored row of the provider, and the dedup key with it.
--
-- :one rather than :execrows because the CTE moves the row count out of the command tag.
-- count(*) over the closed rows is the same int64 the caller already had.
-- Locking and freshness recheck: see CloseUnseenJobs.
WITH cand AS (
    SELECT j.id
    FROM jobs j
    JOIN job_activity a ON a.job_id = j.id
    WHERE j.closed_at IS NULL AND NOT j.is_private
      AND j.source = sqlc.arg(source)
      AND a.last_seen_at < sqlc.arg(cutoff)
      AND j.external_id LIKE sqlc.arg(board_pattern)
      AND (sqlc.arg(url_pattern)::text = '' OR j.url LIKE sqlc.arg(url_pattern)::text)
    ORDER BY j.id
    FOR NO KEY UPDATE OF j, a
), closed AS (
    UPDATE jobs
    SET closed_at     = now(),
        closed_reason = 'unseen',
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

-- name: CloseUnseenJobsForBoards :one
-- CloseUnseenJobsForBoard for every qualifying board of one run in ONE statement, so the
-- board-scoped sweep is one scan of the provider's open rows instead of one statement per
-- board. Per board was the shape until 2026-10-23, and over the crawl host's ~26 ms link to
-- the database it cost ~50 ms a board: a recruitee run (~3,900 boards) spent minutes in the
-- loop, and the unit's timeout cancelled 1,431 of its closes mid-way. Measured on prod the
-- same day, this statement's candidate scan takes under a second for recruitee and ~110 s
-- cold for all of workday's 1.5M open rows — the per-board loop over one workday shard costs
-- about the same, so nothing gets slower and the rest get dramatically faster.
--
-- board_patterns are externalid.BoardPattern values, one per board, exactly as the
-- single-board query takes them; LIKE ANY keeps the per-board predicate's semantics
-- (escaping, the ":" terminator) without a second definition of the namespace. The
-- removal enqueue rides the statement for the same reason it does in every close.
--
-- url_pattern is CloseUnseenJobsForBoard's: '' for every provider but a region-scoped one, whose
-- caller issues one statement per region, all of that region's boards in it.
-- Locking and freshness recheck: see CloseUnseenJobs. The scan joins one primary-key probe on
-- job_activity per open row of the provider: re-EXPLAIN it on workday-sized data.
WITH cand AS (
    SELECT j.id
    FROM jobs j
    JOIN job_activity a ON a.job_id = j.id
    WHERE j.closed_at IS NULL AND NOT j.is_private
      AND j.source = sqlc.arg(source)
      AND a.last_seen_at < sqlc.arg(cutoff)
      AND j.external_id LIKE ANY(sqlc.arg(board_patterns)::text[])
      AND (sqlc.arg(url_pattern)::text = '' OR j.url LIKE sqlc.arg(url_pattern)::text)
    ORDER BY j.id
    FOR NO KEY UPDATE OF j, a
), closed AS (
    UPDATE jobs
    SET closed_at     = now(),
        closed_reason = 'unseen',
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

-- name: CloseRetiredBoardJobs :one
-- Closes the open postings of a board a curator retired (boardcatalog.Retire). Nothing else
-- ever would: a retired board is never crawled again, so no sweep runs for it; Retire
-- deletes its board_health row, which is the only evidence the chronic-board safety net
-- closes from; and its source is still a registered provider, which cmd/liveness leaves to
-- the sweep. aijobs kept 2,569 open postings for eight weeks after its retirement that way
-- (freehire#3796).
--
-- board_pattern is externalid.BoardPattern of the STORED spelling (GetRetiredBoardName). A
-- boardless provider's board is '' and its pattern ':%' matches exactly the ':<id>' keys it
-- stores, nothing a board-scoped sibling stored.
--
-- Both guards ride the statement so it cannot act on a stale read: the board must be retired
-- now, and no live board of the same name may exist in ANY region — external_id carries no
-- region, so while a sibling region is live its postings and the retired one's are
-- indistinguishable, the same refusal the sweep's ambiguousRegionBoards makes.
--
-- closed_reason is 'board_retired' (migration 0212), never 'board_unreachable': a curator
-- deciding a board is gone is a different mechanism from 60 days of failed crawls.
--
-- The search_delete_outbox CTE is copied verbatim from CloseUnseenJobs: the enqueue must ride
-- this statement to stay atomic with the close and exact.
WITH closed AS (
    UPDATE jobs
    SET closed_at     = now(),
        closed_reason = 'board_retired',
        updated_at    = now()
    WHERE closed_at IS NULL
      AND source = sqlc.arg(source)
      AND external_id LIKE sqlc.arg(board_pattern)
      AND EXISTS (
          SELECT 1 FROM boards b
          WHERE b.provider = sqlc.arg(source)
            AND b.board = sqlc.arg(board)
            AND b.region = sqlc.arg(region)
            AND b.status = 'retired'
      )
      AND NOT EXISTS (
          SELECT 1 FROM boards b
          WHERE b.provider = sqlc.arg(source)
            AND lower(b.board) = lower(sqlc.arg(board))
            AND b.status IN ('pending', 'active')
      )
    RETURNING id
), queued AS (
    INSERT INTO search_delete_outbox (job_id)
    SELECT id FROM closed
    ON CONFLICT (job_id) DO NOTHING
)
SELECT count(*) FROM closed;
