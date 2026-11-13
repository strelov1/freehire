-- name: CloseChronicBoardJobs :one
-- The chronic-board safety net (openspec change close-chronically-unreachable-boards, issue
-- #2017), board-scoped: closes every open job of ONE board that board_health.ListChronicBoards
-- has already proven unreachable for the closure window — no per-run cutoff, because the
-- evidence here is weeks of accumulated failure, not one run's coverage. Reuses
-- CloseUnseenJobsForBoard's board_pattern scoping so a same-provider sibling board is never
-- touched.
--
-- closed_reason is 'board_unreachable', distinct from the ordinary sweep's 'unseen', so a job
-- closed here is never conflated with one the per-run sweep closed with actual run coverage —
-- see job-lifecycle spec, "every close records the mechanism".
--
-- The EXISTS clause RE-VALIDATES board_health's current state within this same statement,
-- against the identical chronic predicate ListChronicBoards used to select the row in the
-- first place (caught in review, PR #2641/CodeRabbit): the caller reads chronic boards, then
-- closes them, as two separate round trips, and a board can recover — a real crawl can
-- succeed, calling RecordBoardSuccess — in the gap between the two. Re-checking here rather
-- than trusting what the caller read earlier closes that window: if the board's last_success_at
-- has moved since the caller's read, this EXISTS is false and the UPDATE matches nothing,
-- exactly as if the caller had re-read board_health immediately before closing.
--
-- The search_delete_outbox CTE is copied verbatim from CloseUnseenJobs: the enqueue must ride
-- this statement to stay atomic with the close and exact.
--
-- :one rather than :execrows because the CTE moves the row count out of the command tag.
WITH closed AS (
    UPDATE jobs
    SET closed_at     = now(),
        closed_reason = 'board_unreachable',
        updated_at    = now()
    WHERE closed_at IS NULL
      AND source = sqlc.arg(source)
      AND external_id LIKE sqlc.arg(board_pattern)
      AND EXISTS (
          SELECT 1 FROM board_health bh
          WHERE bh.provider = sqlc.arg(source)
            AND bh.board = sqlc.arg(board)
            AND ((bh.last_success_at IS NOT NULL AND bh.last_success_at < now() - sqlc.arg(age_window)::interval)
              OR (bh.last_success_at IS NULL AND bh.first_seen_at < now() - sqlc.arg(age_window)::interval))
      )
    RETURNING id
), queued AS (
    INSERT INTO search_delete_outbox (job_id)
    SELECT id FROM closed
    ON CONFLICT (job_id) DO NOTHING
)
SELECT count(*) FROM closed;

-- name: CloseBoardGoneJobs :one
-- Close one board only after repeated explicit disappearance spanning age_window and a recent
-- successful sibling crawl. Revalidate current health in the UPDATE to close the read/write gap.
WITH closed AS (
    UPDATE jobs
    SET closed_at = now(), closed_reason = 'board_gone', updated_at = now()
    WHERE closed_at IS NULL
      AND source = sqlc.arg(source)
      AND NOT is_private
      AND external_id LIKE sqlc.arg(board_pattern)
      AND EXISTS (
          SELECT 1 FROM board_health h
          WHERE h.provider = sqlc.arg(source)
            AND h.board = sqlc.arg(board)
            AND h.region = sqlc.arg(region)
            AND h.consecutive_board_gone_failures >= 3
            AND h.board_gone_since < now() - sqlc.arg(age_window)::interval
            AND h.last_error_at >= h.board_gone_since + sqlc.arg(age_window)::interval
            AND NOT EXISTS (
                SELECT 1 FROM board_health regional
                WHERE regional.provider = h.provider
                  AND regional.board = h.board
                  AND regional.region <> h.region
            )
            AND EXISTS (
                SELECT 1 FROM board_health sibling
                WHERE sibling.provider = h.provider
                  AND (sibling.board, sibling.region) <> (h.board, h.region)
                  AND sibling.board <> h.board
                  AND sibling.consecutive_failures = 0
                  AND sibling.last_success_at >= now() - interval '48 hours'
            )
      )
    RETURNING id
), queued AS (
    INSERT INTO search_delete_outbox (job_id)
    SELECT id FROM closed
    ON CONFLICT (job_id) DO NOTHING
)
SELECT count(*) FROM closed;

-- name: CountBoardGoneJobs :one
-- Dry-run twin of CloseBoardGoneJobs with the same current-health predicate.
SELECT count(*) FROM jobs
WHERE closed_at IS NULL
  AND source = sqlc.arg(source)
  AND NOT is_private
  AND external_id LIKE sqlc.arg(board_pattern)
  AND EXISTS (
      SELECT 1 FROM board_health h
      WHERE h.provider = sqlc.arg(source)
        AND h.board = sqlc.arg(board)
        AND h.region = sqlc.arg(region)
        AND h.consecutive_board_gone_failures >= 3
        AND h.board_gone_since < now() - sqlc.arg(age_window)::interval
        AND h.last_error_at >= h.board_gone_since + sqlc.arg(age_window)::interval
        AND NOT EXISTS (
            SELECT 1 FROM board_health regional
            WHERE regional.provider = h.provider
              AND regional.board = h.board
              AND regional.region <> h.region
        )
        AND EXISTS (
            SELECT 1 FROM board_health sibling
            WHERE sibling.provider = h.provider
              AND (sibling.board, sibling.region) <> (h.board, h.region)
              AND sibling.board <> h.board
              AND sibling.consecutive_failures = 0
              AND sibling.last_success_at >= now() - interval '48 hours'
        )
  );

-- name: CloseChronicProviderJobs :one
-- The chronic-board safety net's source-scoped sibling, for a BOARDLESS provider's chronic
-- record (board = '', see ingest-board-health spec): such a record already stands for the
-- provider's whole crawl, so closing "this board's jobs" means the whole provider's open jobs,
-- by source alone — mirrors CloseUnseenJobsBySource but with no per-run cutoff, for the same
-- reason CloseChronicBoardJobs has none. Carries the same board_health re-validation EXISTS
-- clause as CloseChronicBoardJobs, and for the same reason.
WITH closed AS (
    UPDATE jobs
    SET closed_at     = now(),
        closed_reason = 'board_unreachable',
        updated_at    = now()
    WHERE closed_at IS NULL
      AND source = sqlc.arg(source)
      AND EXISTS (
          SELECT 1 FROM board_health bh
          WHERE bh.provider = sqlc.arg(source)
            AND bh.board = ''
            AND ((bh.last_success_at IS NOT NULL AND bh.last_success_at < now() - sqlc.arg(age_window)::interval)
              OR (bh.last_success_at IS NULL AND bh.first_seen_at < now() - sqlc.arg(age_window)::interval))
      )
    RETURNING id
), queued AS (
    INSERT INTO search_delete_outbox (job_id)
    SELECT id FROM closed
    ON CONFLICT (job_id) DO NOTHING
)
SELECT count(*) FROM closed;

-- name: CountChronicBoardJobs :one
-- What CloseChronicBoardJobs would close, for the safety-net worker's dry-run report — same
-- predicate (including the board_health re-validation, so a dry run cannot report a count for
-- a board that has already recovered), no write.
SELECT count(*) FROM jobs
WHERE closed_at IS NULL
  AND source = sqlc.arg(source)
  AND external_id LIKE sqlc.arg(board_pattern)
  AND EXISTS (
      SELECT 1 FROM board_health bh
      WHERE bh.provider = sqlc.arg(source)
        AND bh.board = sqlc.arg(board)
        AND ((bh.last_success_at IS NOT NULL AND bh.last_success_at < now() - sqlc.arg(age_window)::interval)
          OR (bh.last_success_at IS NULL AND bh.first_seen_at < now() - sqlc.arg(age_window)::interval))
  );

-- name: CountChronicProviderJobs :one
-- What CloseChronicProviderJobs would close, for the safety-net worker's dry-run report. Same
-- board_health re-validation as its close counterpart.
SELECT count(*) FROM jobs
WHERE closed_at IS NULL
  AND source = sqlc.arg(source)
  AND EXISTS (
      SELECT 1 FROM board_health bh
      WHERE bh.provider = sqlc.arg(source)
        AND bh.board = ''
        AND ((bh.last_success_at IS NOT NULL AND bh.last_success_at < now() - sqlc.arg(age_window)::interval)
          OR (bh.last_success_at IS NULL AND bh.first_seen_at < now() - sqlc.arg(age_window)::interval))
  );

-- name: CloseEmptyFeedBoardJobs :one
-- The empty-feed safety net, board-scoped: closes every open job of ONE board that
-- board_health.ListEmptyFeedBoards has proven reachable-but-empty for the closure window.
-- Structurally identical to CloseChronicBoardJobs — same board_pattern scoping so a
-- same-provider sibling board is never touched, same search_delete_outbox CTE riding the
-- statement so the index enqueue stays atomic with the close, same :one because that CTE moves
-- the row count out of the command tag — and different in exactly two places: the reason it
-- writes, and the board_health predicate it re-validates.
--
-- closed_reason is 'feed_empty' (migration 0159), which asserts something none of the existing
-- reasons do: every crawl SUCCEEDED and the feed carried nothing. 'board_unreachable' would
-- claim the opposite diagnosis and send an operator looking at our access rather than at the
-- source's inventory.
--
-- The EXISTS clause re-validates board_health's CURRENT state inside this same statement, for
-- the reason CloseChronicBoardJobs states in full: the caller lists boards and closes them as
-- two separate round trips, and a board can recover in the gap — here by a single crawl that
-- reaches one posting and stamps last_yield_at. Re-checking closes that window; note it
-- re-checks last_yield_at specifically, since that is the column a recovery moves.
WITH closed AS (
    UPDATE jobs
    SET closed_at     = now(),
        closed_reason = 'feed_empty',
        updated_at    = now()
    WHERE closed_at IS NULL
      AND source = sqlc.arg(source)
      AND external_id LIKE sqlc.arg(board_pattern)
      AND EXISTS (
          SELECT 1 FROM board_health bh
          WHERE bh.provider = sqlc.arg(source)
            AND bh.board = sqlc.arg(board)
            AND bh.last_yield_at IS NOT NULL
            AND bh.last_yield_at < now() - sqlc.arg(age_window)::interval
            AND bh.last_success_at IS NOT NULL
            AND bh.last_success_at >= now() - sqlc.arg(age_window)::interval
            AND bh.consecutive_failures = 0
      )
    RETURNING id
), queued AS (
    INSERT INTO search_delete_outbox (job_id)
    SELECT id FROM closed
    ON CONFLICT (job_id) DO NOTHING
)
SELECT count(*) FROM closed;

-- name: CloseEmptyFeedProviderJobs :one
-- The empty-feed safety net's source-scoped sibling, for a BOARDLESS provider's record
-- (board = '', see ingest-board-health spec): such a record already stands for the provider's
-- whole crawl, so closing "this board's jobs" means the whole provider's open jobs, by source
-- alone. Mirrors CloseChronicProviderJobs, carrying the same re-validation EXISTS clause as
-- CloseEmptyFeedBoardJobs and for the same reason.
WITH closed AS (
    UPDATE jobs
    SET closed_at     = now(),
        closed_reason = 'feed_empty',
        updated_at    = now()
    WHERE closed_at IS NULL
      AND source = sqlc.arg(source)
      AND EXISTS (
          SELECT 1 FROM board_health bh
          WHERE bh.provider = sqlc.arg(source)
            AND bh.board = ''
            AND bh.last_yield_at IS NOT NULL
            AND bh.last_yield_at < now() - sqlc.arg(age_window)::interval
            AND bh.last_success_at IS NOT NULL
            AND bh.last_success_at >= now() - sqlc.arg(age_window)::interval
            AND bh.consecutive_failures = 0
      )
    RETURNING id
), queued AS (
    INSERT INTO search_delete_outbox (job_id)
    SELECT id FROM closed
    ON CONFLICT (job_id) DO NOTHING
)
SELECT count(*) FROM closed;

-- name: CountEmptyFeedBoardJobs :one
-- What CloseEmptyFeedBoardJobs would close, for the safety-net worker's dry-run report — same
-- predicate including the board_health re-validation, so a dry run cannot report a count for a
-- board that has already yielded again, and no write.
SELECT count(*) FROM jobs
WHERE closed_at IS NULL
  AND source = sqlc.arg(source)
  AND external_id LIKE sqlc.arg(board_pattern)
  AND EXISTS (
      SELECT 1 FROM board_health bh
      WHERE bh.provider = sqlc.arg(source)
        AND bh.board = sqlc.arg(board)
        AND bh.last_yield_at IS NOT NULL
        AND bh.last_yield_at < now() - sqlc.arg(age_window)::interval
        AND bh.last_success_at IS NOT NULL
        AND bh.last_success_at >= now() - sqlc.arg(age_window)::interval
        AND bh.consecutive_failures = 0
  );

-- name: CountEmptyFeedProviderJobs :one
-- What CloseEmptyFeedProviderJobs would close, for the safety-net worker's dry-run report.
-- Same board_health re-validation as its close counterpart.
SELECT count(*) FROM jobs
WHERE closed_at IS NULL
  AND source = sqlc.arg(source)
  AND EXISTS (
      SELECT 1 FROM board_health bh
      WHERE bh.provider = sqlc.arg(source)
        AND bh.board = ''
        AND bh.last_yield_at IS NOT NULL
        AND bh.last_yield_at < now() - sqlc.arg(age_window)::interval
        AND bh.last_success_at IS NOT NULL
        AND bh.last_success_at >= now() - sqlc.arg(age_window)::interval
        AND bh.consecutive_failures = 0
  );

-- name: UnseenJobIDs :many
-- skipped_board_patterns (externalid.BoardPattern of every board the run SKIPPED — in quiet pause
-- or failure cooldown) are excluded: a skipped board was not looked at, so its rows' age since
-- last seen says nothing about them, and a quiet pause outlasts the 48h window. COALESCE because a
-- NULL array would make NOT (LIKE ANY(NULL)) NULL and silently close nothing at all.
-- Same candidate set as CloseUnseenJobs, unmaterialized. The sweep's fallback path
-- (see CloseUnseenJobByID) uses this to close row by row when the single bulk UPDATE
-- fails — e.g. a heap/index-corrupted row aborts the whole batch (2026-08-11 incident:
-- one duplicated jobs_pkey value blocked greenhouse's sweep on every run) — so ids are
-- fetched separately and closed one at a time, letting one bad id be skipped without
-- blocking the rest.
SELECT j.id FROM jobs j
JOIN job_activity a ON a.job_id = j.id
WHERE j.closed_at IS NULL
  AND j.source = sqlc.arg(source)
  AND a.last_seen_at < sqlc.arg(cutoff)
  AND j.company_slug = ANY(sqlc.arg(company_slugs)::text[])
  AND NOT (j.external_id LIKE ANY(COALESCE(sqlc.arg(skipped_board_patterns)::text[], '{}')));

-- name: UnseenJobIDsBySource :many
-- skipped_board_patterns: the boards the run skipped are excluded, as in CloseUnseenJobs.
-- Row-by-row fallback candidate set for CloseUnseenJobsBySource — see UnseenJobIDs.
SELECT j.id FROM jobs j
JOIN job_activity a ON a.job_id = j.id
WHERE j.closed_at IS NULL
  AND j.source = sqlc.arg(source)
  AND a.last_seen_at < sqlc.arg(cutoff)
  AND NOT (j.external_id LIKE ANY(COALESCE(sqlc.arg(skipped_board_patterns)::text[], '{}')));

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
--
-- cutoff is the crawl cutoff the candidates were listed by. The listing and this close are
-- separate statements, so a crawl can list the posting again between them: the close re-checks
-- the activity row's freshness under the same locks as the bulk sweep (CloseUnseenJobs, in
-- jobs_close_unseen.sql: jobs then job_activity, FOR NO KEY UPDATE, recheck after waiting) and
-- writes nothing, queues no removal, for a row that is fresh now. Private postings are excluded
-- from this catalogue close.
WITH cand AS (
    SELECT j.id
    FROM jobs j
    JOIN job_activity a ON a.job_id = j.id
    WHERE j.id = sqlc.arg(id) AND j.closed_at IS NULL AND NOT j.is_private
      AND a.last_seen_at < sqlc.arg(cutoff)
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

-- name: CloseUnseenJobsBySource :one
-- skipped_board_patterns: the boards the run skipped are excluded, as in CloseUnseenJobs.
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
-- Locking and freshness recheck: see CloseUnseenJobs (jobs_close_unseen.sql).
WITH cand AS (
    SELECT j.id
    FROM jobs j
    JOIN job_activity a ON a.job_id = j.id
    WHERE j.closed_at IS NULL AND NOT j.is_private
      AND j.source = sqlc.arg(source)
      AND a.last_seen_at < sqlc.arg(cutoff)
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
