-- Queries used only by the one-off cmd/backfill-link-contributions, which carries the
-- rows #2357 left behind in link_contributions into boards and board_submissions. That
-- change moved the read and write paths but not the data, so 401 contributions from 11
-- users stopped being visible and 28 unprocessed ones stopped being actionable.
--
-- Deleted with the worker once the backfill has run in prod and link_contributions is
-- dropped.

-- name: ListLinkContributionsForBackfill :many
-- Every contribution, oldest first, so the carry preserves submission order. Ordered by
-- id rather than created_at: two rows can share a timestamp, and the id is the sequence
-- the submissions actually arrived in.
SELECT id, submitted_by, url, source, board, status, surface, created_at
FROM link_contributions
ORDER BY id;

-- name: InsertBoardSubmissionAt :execrows
-- Carry one unclassified-URL contribution into board_submissions, KEEPING its original
-- created_at — the ordinary insert defaults to now(), which would restamp a submission
-- from August as today and reorder every user's list. A URL already queued is left alone,
-- so a re-run writes nothing.
INSERT INTO board_submissions (url, submitted_by, surface, created_at)
VALUES (sqlc.arg(url), sqlc.arg(submitted_by), sqlc.arg(surface), sqlc.arg(created_at))
ON CONFLICT (url) DO NOTHING;

-- name: InsertPendingBoardAt :execrows
-- Carry one recognized-but-not-yet-onboarded contribution into boards at status='pending',
-- keeping its original created_at for the same reason as above.
--
-- Guarded by NOT EXISTS rather than ON CONFLICT: boards_identity_key is PARTIAL
-- (WHERE status IN ('pending','active')), so a conflict clause only covers a live row and
-- would happily add a second copy beside a rejected or retired one. Any row under this
-- identity, whatever its status, means the board already reached the catalog — so there is
-- nothing to carry, and a re-run writes nothing.
INSERT INTO boards (provider, board, region, company, url, status, submitted_by, surface, created_at)
SELECT sqlc.arg(provider), sqlc.arg(board), '', sqlc.arg(company), sqlc.arg(url),
       'pending', sqlc.arg(submitted_by), sqlc.arg(surface), sqlc.arg(created_at)
WHERE NOT EXISTS (
    SELECT 1 FROM boards
    WHERE provider = sqlc.arg(provider) AND lower(board) = lower(sqlc.arg(board))
);

-- name: AttributeBoardToSubmitter :execrows
-- Give an existing catalog row the submitter who contributed it. The board reached the
-- catalog through the YAML backfill, which carried no attribution, so the person who found
-- it disappeared from their own contributions list.
--
-- Guarded on submitted_by IS NULL: a row that already names a submitter keeps them, which
-- makes a re-run inert and stops a later duplicate contribution reassigning credit.
UPDATE boards
SET submitted_by = sqlc.arg(submitted_by), url = sqlc.arg(url), surface = sqlc.arg(surface)
WHERE provider = sqlc.arg(provider) AND lower(board) = lower(sqlc.arg(board))
  AND submitted_by IS NULL;
