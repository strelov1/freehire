-- name: ProviderIngestHealth :many
-- Per-provider ingest health: when the provider's most recent board crawl succeeded, and
-- how its boards split across four mutually exclusive states. It is also the ONLY source of
-- the fleet-wide freehire_boards_total, which cmd/queue-metrics sums from these rows, so the
-- per-provider and fleet-wide families cannot disagree about what a board is.
--
-- The population is the CATALOGUE — every board in `boards` that ingest still crawls
-- (status pending or active) — with board_health LEFT JOINed onto it. It used to be
-- board_health alone, and that was wrong in both directions, each time silently:
--
--   * A board the catalogue has never crawled has no board_health row, so a provider whose
--     boards are all new was not exported at all. Measured 2026-10-19: after a re-key, all
--     3,324 gupy boards were `pending` and uncrawled, and gupy had NO series in either
--     provider family — on the day its API had already been dead for 19 days with 73k open
--     postings going stale. Vanishing is how an outage becomes invisible: no alert can be
--     late about a series that is not there. 23,806 live boards across 26 providers had no
--     health row that day (careerplug alone 8,071).
--   * A RETIRED board keeps its board_health row unless the retiring path deletes it, so a
--     retired provider went on being exported. All 5,833 apploi boards were retired on
--     2026-09-15 (cmd/close-apploi-misattributed) and apploi still published 5,833 boards
--     and a last success 40 days old, which a "large provider has gone quiet" rule would
--     page on forever. 6,794 health rows matched no live board that day.
--
-- The join key is board_health's own identity, (provider, lower(board), region) — see
-- migration 0171 — which is also how the catalogue's boards_identity_key folds a live
-- board, so a live board meets at most one health row.
--
-- The four states:
--   uncrawled — in the catalogue, no health row: no crawl of it has ever been recorded.
--   cooled    — in cooldown, so NOT currently being crawled. Takes precedence over
--               failing: a cooled board always has failures behind it (pipeline's
--               CooldownFor), and counting it twice would make the states exceed the fleet.
--   failing   — out of cooldown with consecutive failures.
--   healthy   — out of cooldown, last crawl succeeded.
--
-- last_success_at is max() over the provider's LIVE boards only, so a retired board's old
-- success cannot make a provider look fresher than its current boards are. It is NULL for a
-- provider no live board of which has ever succeeded, and the caller publishes that as an
-- ABSENT sample, never a zero (a Unix zero reads as overdue since 1970).
--
-- first_listed_at is what an alert falls back to in exactly that case: the moment the
-- provider's OLDEST live board entered the catalogue. "Listed since X and never once
-- succeeded" is a lower bound on how long the provider has been producing nothing, so
-- `time() - (last_success or first_listed)` is an honest silence for every provider. The
-- OLDEST board, never the newest: with the newest, adding one board to a provider that is
-- failing would restart its clock and hide the outage for another day. It is
-- boards.created_at rather than board_health.first_seen_at because an uncrawled board has
-- no health row to carry one. It is never NULL (created_at is NOT NULL and every row here
-- has at least one live board). One limit, accepted: a re-key retires the old boards and
-- the new ones start a fresh clock, so silence that began before the re-key is not
-- counted — the re-key is a deliberate act by someone already looking at the provider.
--
-- Cost, measured on prod 2026-10-19 (213k live boards, 196k health rows): 228ms warm, a
-- parallel hash join of the two tables, against 37ms for the board_health-only form it
-- replaces. It also replaces BoardHealthMetrics, the fleet-wide twin it now feeds, so the
-- net cost is one query of this size a minute. Neither table is jobs, whose 8M-row scan is
-- what this file's header exists to forbid.
SELECT
    b.provider,
    -- A board in its quiet pause (migration 0198) skips crawls on purpose for up to 72h; it reads
    -- as fresh while the pause holds, or a provider whose boards are all quiet would trip the
    -- silence alert part of every cycle. The pause is only ever set by a success.
    max(CASE WHEN h.quiet_until > now() THEN now() ELSE h.last_success_at END)::timestamptz AS last_success_at,
    min(b.created_at)::timestamptz      AS first_listed_at,
    count(*) FILTER (
        WHERE h.provider IS NULL
    ) AS uncrawled,
    count(*) FILTER (
        WHERE h.provider IS NOT NULL
          AND h.cooldown_until > now()
    ) AS cooled,
    count(*) FILTER (
        WHERE h.provider IS NOT NULL
          AND (h.cooldown_until IS NULL OR h.cooldown_until <= now())
          AND h.consecutive_failures > 0
    ) AS failing,
    count(*) FILTER (
        WHERE h.provider IS NOT NULL
          AND (h.cooldown_until IS NULL OR h.cooldown_until <= now())
          AND h.consecutive_failures = 0
    ) AS healthy
FROM boards b
LEFT JOIN board_health h
       ON h.provider = b.provider
      AND lower(h.board) = lower(b.board)
      AND h.region = b.region
WHERE b.status IN ('pending', 'active')
GROUP BY b.provider
ORDER BY b.provider;

-- name: IngestDuplicationMetrics :many
-- How many rows each source has just written, and how many DISTINCT postings those rows
-- stand for. The gap between the two is the signal: one posting stored under many boards.
--
-- It exists because nothing could see the apploi failure of 2026-09-02. api.apploi.com
-- stopped honouring its `employer` parameter, so all 5,833 of that provider's boards began
-- fetching the same global catalogue, and 1,565,701 rows accumulated standing for 3,024
-- real jobs -- 518 copies each, under 518 different and mostly wrong employers, 12.5% of
-- everything live search could find. Every crawl SUCCEEDED throughout. board_health stayed
-- green, the per-run family stayed green, and the queue depths stayed normal, because
-- nothing was failing: the platform answered 200 with valid postings. It ran for two weeks
-- and was found by a person reading the catalogue, not by a gauge.
--
-- The window is ten minutes, and that is a measured choice rather than a round number.
-- Measured on prod 2026-09-16: 24h costs 20,818ms, 60m costs 837ms, 15m costs 328ms, and
-- 10m costs 22ms -- the same order as ProviderIngestHealth's 54ms, which is the bar this
-- file's header sets. Ten minutes is also sufficient: the duplication appears BETWEEN
-- boards, never within one crawl (a board that fetches the global catalogue writes each of
-- its postings once), so the gap opens as soon as two boards of the same source land in one
-- window. apploi would have had roughly forty per window. A source whose boards crawl more
-- slowly than that reads 1:1 and is simply not accused.
--
-- Rides jobs_open_created_idx (created_at DESC, id DESC) WHERE closed_at IS NULL, which is
-- why it is scoped to open rows -- and that scope is honest for this question anyway: a row
-- closed within the window was withdrawn, not ingested.
--
-- Publishes the two RAW counts, never a ratio. Which ratio deserves a page belongs in the
-- alert rule, the same argument freehire_provider_boards already makes for board states --
-- and a ratio computed here would also have to invent an answer for a source that wrote
-- nothing, where the honest count is a pair of zeros.
--
-- Sources that write no url are excluded rather than counted as one big duplicate: a row
-- with no url cannot be told apart from another one, so including them would accuse a
-- source of duplication for a field it simply does not carry.
--
-- NOT is_private for the catalogue-wide reason (internal/job/privatejob), and because it is
-- right for this question on its own: a pasted JD was not INGESTED, and counting one would
-- put 'pasted' and 'weblink' in the exposition as providers that no crawl can explain.
SELECT
    j.source                        AS provider,
    count(*)::bigint                AS rows_written,
    count(DISTINCT j.url)::bigint   AS distinct_postings
FROM jobs j
WHERE j.closed_at IS NULL
  AND NOT j.is_private
  AND j.created_at > now() - interval '10 minutes'
  AND j.url <> ''
GROUP BY j.source
ORDER BY j.source;

-- name: SourceQualityMetrics :many
-- Each source's open postings and the five quality counts beside them, as cmd/rollup-stats
-- last measured them (see AggregateOpenJobsBySource and migration 0213), plus when.
--
-- Reads the snapshot, never `jobs`: the measurement is a full scan of the open catalogue,
-- which costs minutes on a loaded host and runs every 3 hours, while this worker runs every
-- minute and must stay a few milliseconds. measured_at is published so a rollup that has
-- stopped running shows up as an ageing snapshot rather than as frozen, plausible counts.
--
-- Its own query rather than ListSourceStats, which serves the public /api/v1/sources and
-- should not change shape for an operator's gauge.
--
-- A row whose counts are NULL predates migration 0213 and has not been rebuilt since: it is
-- skipped rather than published as zeros, which would read as "nothing missing". The
-- rebuild writes all five together, so testing one is enough; the COALESCEs only tell sqlc
-- the columns are non-null here.
SELECT source, open_jobs,
       coalesce(missing_work_mode_jobs, 0)::bigint AS missing_work_mode_jobs,
       coalesce(missing_geo_jobs, 0)::bigint       AS missing_geo_jobs,
       coalesce(missing_company_jobs, 0)::bigint   AS missing_company_jobs,
       coalesce(unseen_7d_jobs, 0)::bigint         AS unseen_7d_jobs,
       coalesce(posted_over_60d_jobs, 0)::bigint   AS posted_over_60d_jobs,
       measured_at
FROM source_stats
WHERE missing_work_mode_jobs IS NOT NULL
ORDER BY source;

-- name: EmploymentBoilerplateCandidates :many
-- Newest open postings written in the last ten minutes, capped, so cmd/queue-metrics
-- can ask jobfacts whether a stored employment_type is carried only by boilerplate.
--
-- The window matches IngestDuplicationMetrics and rides the same open-created index.
-- The cap is 501 rather than unbounded because each row de-TOASTs a description, and
-- this file's bar is a query that can run every minute beside ingest. 500 is what the
-- gauge examines; the extra row is how the worker tells a full window from a capped
-- one without a second scan. Newest first, so a sustained mislabel is in the sample
-- even when the cap binds.
--
-- The phrase list stays in jobfacts. A SQL copy would drift from the matcher the
-- catalogue actually uses.
--
-- NOT is_private: a pasted JD is not catalogue intake (internal/job/privatejob).
SELECT j.employment_type, j.title, j.description
FROM jobs j
WHERE j.closed_at IS NULL
  AND NOT j.is_private
  AND j.created_at > now() - interval '10 minutes'
  AND j.employment_type IN ('part_time', 'full_time', 'contract')
ORDER BY j.created_at DESC, j.id DESC
LIMIT 501;
