-- name: GetBoardCooldown :one
-- The board's current cooldown_until (NULL = eligible). Absent row → pgx.ErrNoRows,
-- which the caller treats as "never seen, eligible". region disambiguates a board id that
-- repeats across independent regional slices (e.g. Adzuna's "it-jobs" once per country); every
-- other provider passes '' here, matching the column's default.
--
-- quiet_until rides along (migration 0198) so the one lookup a crawl already makes per board
-- answers both pauses: a second query per board would double the per-board round trips over
-- the crawl host's link to the database.
SELECT cooldown_until, quiet_until
FROM board_health
WHERE provider = $1 AND board = $2 AND region = $3;

-- name: RecordBoardSuccess :one
-- A successful crawl clears the failure state and stamps freshness. Upsert so a
-- first-ever crawl creates the row.
--
-- The conflict target is (provider, lower(board), region) — board_health's identity
-- since migration 0171, matching boards_identity_key on the `boards` catalog itself —
-- so a board id that changes case at the provider converges onto its EXISTING row
-- instead of inserting a stale twin. `board = EXCLUDED.board` is what makes that
-- convergence real: without it the row would keep whichever casing it was first
-- created under forever, and every later exact-match lookup by the provider's CURRENT
-- casing (GetBoardCooldown, SetBoardCooldown, DeleteBoardHealth, ClearProviderCooldowns
-- — all below) would stop finding the row.
--
-- `reached` stamps last_yield_at (migration 0158) and is the caller's answer to a question
-- last_ingested_count cannot: whether the crawl actually found a posting on this board. The two
-- differ in both directions. A board can ingest 0 having reached plenty — every posting rejected
-- by the non-tech gate, or already covered by a non-aggregator source, which is the ordinary
-- outcome on an aggregator board whose employers we also crawl directly. And a board can ingest 0
-- having reached nothing at all, which is the empty feed this column exists to detect. Stamping
-- on `last_ingested_count > 0` instead would call the first case empty and eventually close a
-- perfectly live board's jobs.
--
-- On a run that did NOT reach a posting the column is left exactly as it was, rather than
-- cleared: it is a high-water mark, and how long ago it was set is the whole measurement.
--
-- `fingerprint` drives the quiet-board backoff (migration 0198, pipeline.QuietFor). NULL means
-- this crawl cannot vouch for the board's whole listing (interrupted, partly failed, or a
-- self-closing provider whose emissions are changes, not a listing) and leaves all three quiet
-- columns exactly as they were. Equal to the stored fingerprint: one more quiet crawl, and the
-- caller sets the longer pause from the returned streak (SetBoardQuietUntil). Anything else —
-- a posting appeared or disappeared, or no fingerprint was stored yet — records the new one and
-- ends the quiet state. Every board_health.* reference below reads the row BEFORE this update.
INSERT INTO board_health (provider, board, region, consecutive_failures, cooldown_until,
                          last_success_at, last_ingested_count, last_run_at, last_yield_at,
                          listing_fingerprint)
VALUES ($1, $2, $3, 0, NULL, now(), $4, now(),
        CASE WHEN sqlc.arg(reached)::boolean THEN now() END,
        sqlc.narg(fingerprint)::text)
ON CONFLICT (provider, lower(board), region) DO UPDATE SET
    board                = EXCLUDED.board,
    consecutive_failures = 0,
    cooldown_until       = NULL,
    last_success_at      = now(),
    last_ingested_count  = EXCLUDED.last_ingested_count,
    last_run_at          = now(),
    last_yield_at        = CASE WHEN sqlc.arg(reached)::boolean THEN now()
                                ELSE board_health.last_yield_at END,
    quiet_streak         = CASE WHEN sqlc.narg(fingerprint)::text IS NULL THEN board_health.quiet_streak
                                WHEN board_health.listing_fingerprint = sqlc.narg(fingerprint)::text
                                    THEN board_health.quiet_streak + 1
                                ELSE 0 END,
    quiet_until          = CASE WHEN sqlc.narg(fingerprint)::text IS NULL
                                  OR board_health.listing_fingerprint = sqlc.narg(fingerprint)::text
                                    THEN board_health.quiet_until
                                ELSE NULL END,
    listing_fingerprint  = COALESCE(sqlc.narg(fingerprint)::text, board_health.listing_fingerprint)
RETURNING quiet_streak;

-- name: RecordBoardFailure :one
-- Count a failed crawl: bump consecutive_failures, record the error, stamp the run,
-- and RETURN the new failure count so the caller can compute the cooldown (the backoff
-- policy lives in Go, not here). The cooldown itself is applied by SetBoardCooldown.
--
-- Conflict target and board = EXCLUDED.board: same reasoning as RecordBoardSuccess above.
INSERT INTO board_health (provider, board, region, consecutive_failures, last_error, last_error_at, last_run_at)
VALUES ($1, $2, $3, 1, $4, now(), now())
ON CONFLICT (provider, lower(board), region) DO UPDATE SET
    board                = EXCLUDED.board,
    consecutive_failures = board_health.consecutive_failures + 1,
    last_error           = EXCLUDED.last_error,
    last_error_at        = now(),
    last_run_at          = now()
RETURNING consecutive_failures;

-- name: SetBoardCooldown :execrows
-- Apply the Go-computed cooldown window to a board (called only when the backoff
-- policy says to cool down).
--
-- Guarded by the consecutive_failures value the cooldown was computed FROM (the count
-- RecordBoardFailure just returned), not merely the board's identity: the pipeline's worker
-- pool can process the same board twice in one run, so two concurrent RecordFailure calls can
-- race between their own RecordBoardFailure and this UPDATE. Without the guard, whichever
-- SetBoardCooldown lands last wins regardless of which failure count is newer — an earlier,
-- shorter cooldown can overwrite a later, longer one, or this call can clobber a cooldown a
-- concurrent RecordSuccess/ClearProviderCooldowns just cleared. Zero rows affected means a
-- newer writer already moved consecutive_failures past what this cooldown was computed from —
-- expected under the race, not an error, and the caller applies nothing further.
UPDATE board_health
SET cooldown_until = $4
WHERE provider = $1 AND board = $2 AND region = $3 AND consecutive_failures = $5;

-- name: SetBoardQuietUntil :execrows
-- Apply the Go-computed quiet pause (pipeline.QuietFor) to a board whose listing was just found
-- unchanged again. Guarded by the quiet_streak the pause was computed FROM, for the race
-- SetBoardCooldown documents: the worker pool can record the same board twice in one run, and a
-- stale writer must not set a pause over a streak a newer RecordBoardSuccess already reset.
-- Zero rows affected is that race, not an error.
UPDATE board_health
SET quiet_until = $4
WHERE provider = $1 AND board = $2 AND region = $3 AND quiet_streak = $5;

-- name: DeleteBoardHealth :execrows
-- Drop a board's health row entirely, for a board just retired from the catalog: it will
-- never be crawled again, so no cooldown or failure count of its own could ever clear the
-- normal way (a successful crawl), and leaving the row would show it as permanently
-- unhealthy on the public /status page (ProviderHealthRollup, ListUnhealthyBoards) forever.
DELETE FROM board_health
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

-- name: ListChronicBoards :many
-- The boards that have proven unreachable for at least `age_window`, not merely cooled down from
-- a recent run of failures (openspec change close-chronically-unreachable-boards, issue #2017).
-- A board that has succeeded at least once is measured from last_success_at; one that never has
-- is measured from first_seen_at instead, since last_error_at is overwritten every failed run
-- and cannot answer "how long has this been broken". Called with two different windows: a
-- shorter one for the operator-facing chronic report, a longer one that gates the safety-net
-- close (see job-lifecycle spec) — one query, no duplicated threshold logic between the two.
-- Ordered oldest-evidence-first, so both callers see the worst boards first without a second
-- sort. max_boards mirrors ListUnhealthyBoards's cap (see cmd/ingest's unhealthyBoardsCap,
-- freehire 2026-08-14 incident): the per-run report passes a small one, the safety-net closer
-- passes a generous one large enough that a real chronic backlog is never silently truncated —
-- the two callers' needs differ, so one query with a caller-supplied cap serves both rather
-- than duplicating the threshold logic across an uncapped and a capped variant. total is the
-- FULL count before the cap, same convention as ListUnhealthyBoards.Total.
--
-- No differently-cased-twin guard here any more: board_health's identity is
-- case-insensitive as of migration 0171 (board_health_identity_key), so two rows for the
-- same (provider, board, region) that differ only by case cannot exist — the schema
-- makes the twin this query used to filter out impossible to create, rather than this
-- query hiding it after the fact.
SELECT provider, board, region, consecutive_failures, cooldown_until, last_error, last_error_at,
       last_success_at, first_seen_at, count(*) OVER () AS total
FROM board_health h
WHERE (last_success_at IS NOT NULL AND last_success_at < now() - sqlc.arg(age_window)::interval)
    OR (last_success_at IS NULL AND first_seen_at < now() - sqlc.arg(age_window)::interval)
ORDER BY coalesce(last_success_at, first_seen_at), provider, board, region
LIMIT sqlc.arg(max_boards);

-- name: ListEmptyFeedBoards :many
-- The boards whose crawls SUCCEED but whose feed has carried nothing for at least `age_window`
-- — the exact twin of ListChronicBoards, measuring the failure mode that one structurally
-- cannot see (migration 0158 records the incident this answers).
--
-- Three conditions, and each excludes a different thing that must not be closed as "empty":
--
--  1. last_yield_at IS NOT NULL. NULL means no yield has been OBSERVED since the column was
--     added, which is not the same claim as "this feed is empty" — and on the day 0158 landed
--     it was true of every row in the table. Treating it as evidence is the one mistake here
--     that would close the whole catalogue, so NULL never qualifies, permanently. A board that
--     truly yields nothing from now on gets its seeded stamp from the migration and ages out
--     of the window on its own.
--  2. last_yield_at older than the window. The measurement itself.
--  3. the board is REACHABLE: it has succeeded at least once inside the same window, and its
--     most recent crawl succeeded (consecutive_failures = 0). Without this an unreachable
--     board qualifies here too — its last_yield_at is old for the entirely different reason
--     that nothing has been able to read it — and it would be closed under the wrong reason,
--     hiding a broken adapter behind a diagnosis that points at the source's inventory.
--     Requiring a CURRENT clean crawl rather than merely a recent one costs only a cycle's
--     latency on a board that is genuinely empty and transiently failing.
--
-- Ordered oldest-evidence-first and capped by max_boards, same convention as ListChronicBoards;
-- total is the FULL count before the cap.
SELECT provider, board, region, last_success_at, last_yield_at, first_seen_at,
       count(*) OVER () AS total
FROM board_health
WHERE last_yield_at IS NOT NULL
  AND last_yield_at < now() - sqlc.arg(age_window)::interval
  AND last_success_at IS NOT NULL
  AND last_success_at >= now() - sqlc.arg(age_window)::interval
  AND consecutive_failures = 0
ORDER BY last_yield_at, provider, board, region
LIMIT sqlc.arg(max_boards);

-- name: CountBoardHealthRegions :one
-- How many board_health rows exist for one (provider, board) name across every region it has
-- ever been seen under. More than one means the name is REGION-AMBIGUOUS — the `boards`
-- catalog allows one board name to repeat under a provider, distinguished only by region (e.g.
-- Adzuna's "it-jobs" once per country, internal/ingest/sources/adzuna.go), but
-- jobs.external_id carries no region dimension at all (externalid.Namespace(board, id)), so a
-- board-scoped `external_id LIKE '<board>:%'` close cannot tell one region's postings from
-- another's. The ordinary per-run sweep already refuses to board-scope such a name
-- (pipeline.ambiguousRegionBoards); the chronic-board safety net (cmd/close-chronic-boards)
-- makes the same check against board_health directly, since it has no crawl-run board list to
-- consult and does not need one — board_health's own composite key already records every
-- region a board name has ever been crawled under.
SELECT count(*) FROM board_health WHERE provider = sqlc.arg(provider) AND board = sqlc.arg(board);

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
-- yields 0, not NULL). A provider with no ACTIVE board is left out: board_health rows
-- outlive the boards they describe, so a provider whose boards were all retired or rejected
-- (apploi's 5,833, retired 2026-09-15) would otherwise read "down" forever for a crawl
-- nobody runs any more.
SELECT
    provider,
    count(*)                                                         AS total_boards,
    count(*) FILTER (WHERE cooldown_until IS NULL OR cooldown_until <= now()) AS healthy_boards,
    count(*) FILTER (WHERE cooldown_until IS NOT NULL AND cooldown_until > now()) AS cooled_boards,
    max(last_run_at)::timestamptz                                    AS last_run_at,
    -- A board in its quiet pause (migration 0198) is not crawled for up to 72h on purpose, which
    -- outlasts the page's 48h freshness; it reads as fresh while the pause holds, since the pause
    -- is only ever set by a success.
    max(CASE WHEN quiet_until > now() THEN now() ELSE last_success_at END)::timestamptz AS last_success_at,
    coalesce(sum(last_ingested_count) FILTER (WHERE consecutive_failures = 0), 0)::bigint AS ingested_total
FROM board_health
WHERE EXISTS (
    SELECT 1 FROM boards b WHERE b.provider = board_health.provider AND b.status = 'active'
)
GROUP BY provider
ORDER BY provider;

-- name: ListBoardLastRuns :many
-- When each of a provider's boards was last crawled, for cmd/ingest to start the boards it has
-- waited longest on (see orderOldestRunFirst). A board absent here has never been crawled at all.
-- Only the three columns the ordering reads: this runs once per ingest run over a provider that
-- can hold tens of thousands of boards.
SELECT board, region, last_run_at
FROM board_health
WHERE provider = $1;
