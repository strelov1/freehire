-- name: GetBoardCooldown :one
-- The board's current cooldown_until (NULL = eligible). Absent row → pgx.ErrNoRows,
-- which the caller treats as "never seen, eligible". region disambiguates a board id that
-- repeats across independent regional slices (e.g. Adzuna's "it-jobs" once per country); every
-- other provider passes '' here, matching the column's default.
SELECT cooldown_until
FROM board_health
WHERE provider = $1 AND board = $2 AND region = $3;

-- name: RecordBoardSuccess :exec
-- A successful crawl clears the failure state and stamps freshness. Upsert so a
-- first-ever crawl creates the row.
INSERT INTO board_health (provider, board, region, consecutive_failures, cooldown_until,
                          last_success_at, last_ingested_count, last_run_at)
VALUES ($1, $2, $3, 0, NULL, now(), $4, now())
ON CONFLICT (provider, board, region) DO UPDATE SET
    consecutive_failures = 0,
    cooldown_until       = NULL,
    last_success_at      = now(),
    last_ingested_count  = EXCLUDED.last_ingested_count,
    last_run_at          = now();

-- name: RecordBoardFailure :one
-- Count a failed crawl: bump consecutive_failures, record the error, stamp the run,
-- and RETURN the new failure count so the caller can compute the cooldown (the backoff
-- policy lives in Go, not here). The cooldown itself is applied by SetBoardCooldown.
INSERT INTO board_health (provider, board, region, consecutive_failures, last_error, last_error_at, last_run_at)
VALUES ($1, $2, $3, 1, $4, now(), now())
ON CONFLICT (provider, board, region) DO UPDATE SET
    consecutive_failures = board_health.consecutive_failures + 1,
    last_error           = EXCLUDED.last_error,
    last_error_at        = now(),
    last_run_at          = now()
RETURNING consecutive_failures;

-- name: SetBoardCooldown :exec
-- Apply the Go-computed cooldown window to a board (called only when the backoff
-- policy says to cool down).
UPDATE board_health
SET cooldown_until = $4
WHERE provider = $1 AND board = $2 AND region = $3;

-- name: ListUnhealthyBoards :many
-- The worst $1 boards currently failing or cooled down, worst first — the source of the
-- per-run summary log. Every row also carries the FULL unhealthy count: count(*) OVER () is
-- evaluated over the whole filtered set, before the LIMIT, so the caller reports how many
-- boards are broken without a second round trip and without naming them all. Ask this table
-- directly for the rest.
SELECT provider, board, region, consecutive_failures, cooldown_until, last_error, last_error_at,
       count(*) OVER () AS total
FROM board_health
WHERE consecutive_failures > 0 OR (cooldown_until IS NOT NULL AND cooldown_until > now())
ORDER BY consecutive_failures DESC, provider, board, region
LIMIT sqlc.arg(max_boards);

-- name: ListCooledBoards :many
-- Up to $2 (board, region) pairs currently in an active cooldown for a provider,
-- soonest-to-expire first — the recovery probe's candidates. The ordering rotates the sample as
-- cooldowns lapse, so a run does not keep probing the same few boards.
SELECT board, region
FROM board_health
WHERE provider = $1 AND cooldown_until IS NOT NULL AND cooldown_until > now()
ORDER BY cooldown_until, board, region
LIMIT $2;

-- name: ClearProviderCooldowns :execrows
-- Clear the active cooldown and failure count for every currently-cooled board of a
-- provider — applied once a recovery probe proves the provider reachable again, so the
-- run crawls them this cycle instead of each waiting out its own backoff (up to a day)
-- after a resolved provider-wide outage. Returns the number of boards cleared.
UPDATE board_health
SET cooldown_until = NULL, consecutive_failures = 0
WHERE provider = $1 AND cooldown_until IS NOT NULL AND cooldown_until > now();

-- name: ProviderHealthRollup :many
-- Per-provider health rollup that backs the public /status page: one row per
-- provider with board counts and freshness. Read-only — it never touches cooldown
-- state. healthy_boards counts boards being served (NOT in an active cooldown), so a
-- board that merely erred once but is still crawled every cycle counts as healthy and
-- only a board the backoff actually sidelined is unhealthy; healthy_boards + cooled_boards
-- always equals total_boards. Aggregate-only: it selects no board identifier and no error
-- text, so the public endpoint built on it cannot leak internal detail. ingested_total is
-- coalesced/cast to bigint so it reads as a plain int64 (an all-failing provider
-- yields 0, not NULL).
SELECT
    provider,
    count(*)                                                         AS total_boards,
    count(*) FILTER (WHERE cooldown_until IS NULL OR cooldown_until <= now()) AS healthy_boards,
    count(*) FILTER (WHERE cooldown_until IS NOT NULL AND cooldown_until > now()) AS cooled_boards,
    max(last_run_at)::timestamptz                                    AS last_run_at,
    max(last_success_at)::timestamptz                                AS last_success_at,
    coalesce(sum(last_ingested_count) FILTER (WHERE consecutive_failures = 0), 0)::bigint AS ingested_total
FROM board_health
GROUP BY provider
ORDER BY provider;
