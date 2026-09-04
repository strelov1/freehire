-- name: InsertBoard :one
-- Insert a board row. Callers that pass status='active' also pass a non-null
-- activated_at (a curator addition via cmd/add-board); every other caller passes NULL,
-- for both status='pending' and status='rejected'. A collision with an existing
-- 'pending'/'active' row on (provider, lower(board), region) — boards_identity_key — fails as a unique violation;
-- the caller maps that to a duplicate-board error.
INSERT INTO boards (provider, board, region, company, hub, tenants, url, status,
                     submitted_by, surface, rejected_reason, activated_at)
VALUES (sqlc.arg(provider), sqlc.arg(board), sqlc.arg(region), sqlc.arg(company),
        sqlc.arg(hub), sqlc.arg(tenants), sqlc.arg(url), sqlc.arg(status),
        sqlc.arg(submitted_by), sqlc.arg(surface), sqlc.arg(rejected_reason),
        sqlc.arg(activated_at))
RETURNING *;

-- name: ActivateBoard :execrows
-- Flip a board's first successful crawl from pending to active. A no-op (0 rows) when
-- the board is already active or does not exist, so the caller need not check first.
UPDATE boards
SET status = 'active', activated_at = now()
WHERE provider = $1 AND lower(board) = lower($2) AND region = $3 AND status = 'pending';

-- name: RetireBoard :execrows
-- Retire a live (pending or active) board without deleting its row.
UPDATE boards
SET status = 'retired'
WHERE provider = $1 AND lower(board) = lower($2) AND region = $3
  AND status IN ('pending', 'active');

-- name: ListActiveBoardsForProvider :many
-- The boards cmd/ingest crawls for one provider: pending (unproven, still crawled) and
-- active. Ordered by id for a stable crawl order across runs.
SELECT * FROM boards
WHERE provider = sqlc.arg(provider) AND status IN ('pending', 'active')
ORDER BY id;

-- name: ListLiveBoards :many
-- Every board a crawl still visits, across all providers — the identity cmd/prune needs
-- to decide whether a posting is re-crawlable. Only (provider, board, region), not the
-- whole row: the guard asks a set-membership question and nothing else.
SELECT provider, board, region FROM boards
WHERE status IN ('pending', 'active')
ORDER BY provider, board, region;

-- name: UpdateBoardCompany :execrows
-- Correct a board's company name — for a crowdsourced row seeded with
-- boardcatalog.PlaceholderCompany, once a curator knows the real one. Matches any status
-- (a placeholder is worth fixing whether the board is still pending or already active).
UPDATE boards
SET company = sqlc.arg(company)
WHERE provider = sqlc.arg(provider) AND lower(board) = lower(sqlc.arg(board))
  AND region = sqlc.arg(region) AND status IN ('pending', 'active');

-- name: ListBoardsBySubmitter :many
-- One user's crowdsourced boards, newest first — half of the "my contributions" list
-- (board_submissions holds the other half, the unclassified-URL rows).
SELECT * FROM boards
WHERE submitted_by = sqlc.arg(submitted_by)
ORDER BY created_at DESC;
