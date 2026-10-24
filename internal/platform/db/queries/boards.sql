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

-- name: UpdateBoardCompany :execrows
-- Correct a board's company name — for a crowdsourced row seeded with
-- boardcatalog.PlaceholderCompany, once a curator knows the real one. Matches any status
-- (a placeholder is worth fixing whether the board is still pending or already active).
UPDATE boards
SET company = sqlc.arg(company)
WHERE provider = sqlc.arg(provider) AND lower(board) = lower(sqlc.arg(board))
  AND region = sqlc.arg(region) AND status IN ('pending', 'active');

-- name: RenameSlugBoardCompany :execrows
-- Carry a display name cmd/backfill-company-names resolved back to the board it came
-- from. An adapter copies boards.company onto every posting and the job upsert writes
-- `company = EXCLUDED.company`, so a rename applied to jobs alone is undone by the
-- board's next crawl. Every region and status: a paused board that comes back must not
-- bring the slug with it. Also carries the second pass's names, for a board whose company was
-- its own board id (ListPlaceholderCompanyBoards). Guarded on the slug it replaces, so a name a curator set
-- (or a board whose row names a different company than its postings — see
-- boardcatalog/AGENTS.md) is left alone.
UPDATE boards
SET company = sqlc.arg(name)
WHERE provider = sqlc.arg(provider) AND lower(board) = lower(sqlc.arg(board))
  AND company = sqlc.arg(old_name);

-- name: ListPlaceholderCompanyBoards :many
-- Live boards whose company is still the board id itself — what a harvest writes when it
-- never resolved the tenant's display name, so the id ("<host>/<site>", a GUID) becomes the
-- employer's public name on every posting. cmd/backfill-company-names resolves the real name
-- from the ATS by that exact id and writes it back through RenameSlugBoardCompany. Limited to
-- the providers that have a resolver, since a board no resolver can read is not work.
SELECT provider, board, company
FROM boards
WHERE provider = ANY(sqlc.arg(providers)::text[])
  AND status IN ('pending', 'active')
  AND lower(company) = lower(board)
ORDER BY provider, id;

-- name: ListBoardsBySubmitter :many
-- One user's crowdsourced boards, newest first — half of the "my contributions" list
-- (board_submissions holds the other half, the unclassified-URL rows).
SELECT * FROM boards
WHERE submitted_by = sqlc.arg(submitted_by)
ORDER BY created_at DESC;

-- name: RetireProviderBoards :execrows
-- Retire EVERY live board of one provider in one statement.
--
-- Separate from RetireBoard rather than looping it: this is used when the provider itself is
-- withdrawn, not when a board is found dead, and the populations differ by three orders of
-- magnitude — apploi alone carries 5833 boards (migration 0165). A loop would be 5833 round
-- trips to say one thing.
--
-- Deliberately does NOT touch 'rejected': a rejected board failed insert-time validation and
-- never became live, so calling it retired would erase why it is there. Idempotent — the
-- status predicate means a re-run matches nothing.
UPDATE boards
SET status = 'retired'
WHERE provider = sqlc.arg(provider)
  AND status IN ('pending', 'active');
