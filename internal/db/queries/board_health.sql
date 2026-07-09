-- name: GetBoardCooldown :one
-- The board's current cooldown_until (NULL = eligible). Absent row → pgx.ErrNoRows,
-- which the caller treats as "never seen, eligible".
SELECT cooldown_until
FROM board_health
WHERE provider = $1 AND board = $2;

-- name: RecordBoardSuccess :exec
-- A successful crawl clears the failure state and stamps freshness. Upsert so a
-- first-ever crawl creates the row.
INSERT INTO board_health (provider, board, consecutive_failures, cooldown_until,
                          last_success_at, last_ingested_count, last_run_at)
VALUES ($1, $2, 0, NULL, now(), $3, now())
ON CONFLICT (provider, board) DO UPDATE SET
    consecutive_failures = 0,
    cooldown_until       = NULL,
    last_success_at      = now(),
    last_ingested_count  = EXCLUDED.last_ingested_count,
    last_run_at          = now();

-- name: RecordBoardFailure :one
-- Count a failed crawl: bump consecutive_failures, record the error, stamp the run,
-- and RETURN the new failure count so the caller can compute the cooldown (the backoff
-- policy lives in Go, not here). The cooldown itself is applied by SetBoardCooldown.
INSERT INTO board_health (provider, board, consecutive_failures, last_error, last_error_at, last_run_at)
VALUES ($1, $2, 1, $3, now(), now())
ON CONFLICT (provider, board) DO UPDATE SET
    consecutive_failures = board_health.consecutive_failures + 1,
    last_error           = EXCLUDED.last_error,
    last_error_at        = now(),
    last_run_at          = now()
RETURNING consecutive_failures;

-- name: SetBoardCooldown :exec
-- Apply the Go-computed cooldown window to a board (called only when the backoff
-- policy says to cool down).
UPDATE board_health
SET cooldown_until = $3
WHERE provider = $1 AND board = $2;

-- name: ListUnhealthyBoards :many
-- Every board currently failing or cooled down, worst first — the operator's
-- "what's broken" query and the source of the per-run summary log.
SELECT provider, board, consecutive_failures, cooldown_until, last_error, last_error_at
FROM board_health
WHERE consecutive_failures > 0 OR (cooldown_until IS NOT NULL AND cooldown_until > now())
ORDER BY consecutive_failures DESC, provider, board;
