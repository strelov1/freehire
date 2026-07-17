-- name: CreateContribution :one
-- Record a contribution of a novel company board. The UNIQUE (source, board) constraint
-- rejects a second contribution of the same board (another vacancy or the listing); the
-- repository maps that unique violation to ErrBoardAlreadyContributed. Runs in the same
-- transaction as IncrementUserPoints.
INSERT INTO link_contributions (submitted_by, url, source, board)
VALUES (sqlc.arg(submitted_by)::bigint, sqlc.arg(url), sqlc.arg(source), sqlc.arg(board))
RETURNING *;

-- name: JobsExistForBoard :one
-- Whether the catalogue already crawls this board — any job whose external_id is prefixed by
-- "<board>:" for the multi-tenant sources. Used to reject a board we already track before any
-- write. starts_with is a plain prefix test — no LIKE wildcards to escape.
SELECT EXISTS (
    SELECT 1 FROM jobs WHERE source = sqlc.arg(source) AND starts_with(external_id, sqlc.arg(board_prefix))
) AS exists;

-- name: ListContributionsByUser :many
-- The "my contributions" list: one user's contributions, newest first.
SELECT * FROM link_contributions
WHERE submitted_by = $1
ORDER BY created_at DESC;

-- name: IncrementUserPoints :exec
-- Award one point to the contributor. Runs in the same transaction as CreateContribution,
-- so a rolled-back insert (e.g. a duplicate-board race) never credits a point.
UPDATE users SET points = points + 1 WHERE id = $1;
