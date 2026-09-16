-- name: DeleteAllJobDailyStats :exec
-- First half of the atomic rebuild: clear the rollup. Run in the same transaction
-- as RebuildJobDailyStats so readers never see an empty table and reopen-orphaned
-- days (a day that had only closures, now reopened) are dropped rather than left
-- stale.
DELETE FROM job_daily_stats;

-- name: RebuildJobDailyStats :execrows
-- Second half of the atomic rebuild: recompute every active day from jobs. `added`
-- counts jobs by their created_at day; `removed` counts jobs by their CURRENT
-- closed_at day (NULL = still open, excluded). Days are UTC calendar dates
-- (AT TIME ZONE 'UTC') so buckets are stable regardless of session timezone. The
-- FULL OUTER JOIN yields one row per day that saw either an add or a removal.
INSERT INTO job_daily_stats (day, added, removed, computed_at)
SELECT
    COALESCE(a.day, r.day)  AS day,
    COALESCE(a.n, 0)::int   AS added,
    COALESCE(r.n, 0)::int   AS removed,
    now()
FROM (
    SELECT (created_at AT TIME ZONE 'UTC')::date AS day, count(*) AS n
    FROM jobs
    GROUP BY 1
) a
FULL OUTER JOIN (
    SELECT (closed_at AT TIME ZONE 'UTC')::date AS day, count(*) AS n
    FROM jobs
    WHERE closed_at IS NOT NULL
    GROUP BY 1
) r ON a.day = r.day;

-- name: ListUserGrowth :many
-- Dense cumulative member-growth series: one UTC calendar day per row from the
-- first registration through today, each carrying the running total of members
-- registered on or before that day, plus that day's own (non-cumulative)
-- new-signup count for the "new members per day" chart. A daily generate_series
-- builds the gap-free calendar (days with no new signups repeat the previous
-- total and carry new=0), the LEFT JOIN attaches each day's new-signup count, and
-- the window SUM makes the running total cumulative, so it is monotonically
-- non-decreasing. Aggregate only — no user identifier, email, or other personal
-- field is selected. With no members the series is empty (min(day) is NULL, so
-- generate_series yields no rows).
WITH daily AS (
    SELECT (created_at AT TIME ZONE 'UTC')::date AS day, count(*) AS n
    FROM users
    GROUP BY 1
)
SELECT
    d::date AS day,
    sum(COALESCE(daily.n, 0)) OVER (ORDER BY d)::int AS total,
    COALESCE(daily.n, 0)::int AS new
FROM generate_series(
    (SELECT min(day) FROM daily),
    (now() AT TIME ZONE 'UTC')::date,
    interval '1 day'
) AS d
LEFT JOIN daily ON daily.day = d::date
ORDER BY d;

-- name: GetEngagementStats :one
-- Aggregate interaction counts for the public engagement endpoint. Aggregate-only:
-- every column is a scalar total, so no user identifier or row-level field is
-- selected. saved / applied are user_jobs interaction-row totals across all users.
-- "viewed" is the human view total (anonymous + signed-in, every visitor) produced by
-- the nginx-log aggregation worker. It sums the worker's per-day rollup
-- (job_daily_views), NOT jobs.view_count — a SUM over the 6M-row jobs table seqscans
-- for ~90s and times the endpoint out. (The per-job "N views" on the job card still
-- reads jobs.view_count directly, no scan.)
--
-- **The rollup is not small.** This comment used to call it "small and fast", which was
-- wrong and expensive: measured 2026-09-16 it holds 9,944,162 rows in 843 MB, carries
-- exactly one index (the `(day, job_id)` primary key) and grows by a row per job per
-- day forever. Both figures below therefore come out of ONE aggregate pass, in a CTE,
-- and a second pass over this table must not be added. Reading it twice is what made
-- /open unusable the day `viewed_since` landed: as its own scalar subquery,
-- `min(day) WHERE page_uniques > 0` planned an ascending walk of the primary key that
-- heap-fetched every row to test an unindexed column, and since the ~4.7M rows before
-- 2026-09-04 all hold a `page_uniques` of 0 it discarded every one of them before the
-- first match — 3,104 ms and 5,033,538 buffer touches, on top of the sum's own 542 ms.
-- The endpoint went from ~0.5s to 3.9-4.4s, /open's slowest leg by a factor of six,
-- and on a host whose crawl fleet already saturates the disk the cold-cache case
-- reached 16s. Folded into one pass it is 64k buffer touches, 78x fewer.
--
-- The seam this leaves: one pass is still a full scan of a table that only grows, so
-- these two figures belong in a published snapshot (`cmd/rollup-stats`, the way
-- `/stats/catalog` is fed — "never count on a request path") before the scan grows
-- back into the same problem. Not built yet because one pass restored the endpoint to
-- its peers (~0.6s, the same as /stats/facets), and that is where the need stops today.
-- It sums `page_uniques`, NEVER `uniques` — the same rule social-digest's ranking
-- follows, and for the same reason: `uniques` fuses bot-filtered page opens with
-- UNFILTERED API reads, and crawlers are most of this host's traffic. Measured
-- 2026-09-16, `uniques` reported 11,027,722 against `page_uniques`' 5,401,347, so
-- the figure this endpoint published was more than half robots — and it sat on /open
-- beside the seven signed-in counters as though it described the same people.
--
-- `viewed` is therefore NOT an all-time figure, and `viewed_since` is what says so.
-- Migration 0138 added `page_uniques` with a `DEFAULT 0` and deliberately did not
-- backfill it, so every row the worker wrote before that day carries a zero: measured
-- 2026-09-16, July held 203,781 `uniques` and August 5,194,396, both against a
-- `page_uniques` of 0. Neither is recoverable. `cmd/rollup-views --backfill` reads the
-- older .gz history, but `processed_view_logs` marks a file applied by its filesystem
-- identity and skips it forever after — which is the same cursor that stops a re-run
-- double-counting `uniques` — and the host's logrotate keeps 12 days, so the July and
-- August logs are gone from disk regardless. Publishing the sum as a cumulative total
-- would trade "inflated by robots" for "silently the last fortnight", so the window is
-- published beside the number instead. It is DERIVED (the earliest day the column
-- actually carries a count), never a constant naming the migration: if the history is
-- ever recovered the window widens on its own, and a hardcoded date would then lie in
-- the other direction. NULL means nothing has been rolled up yet.
--
-- The remaining five mirror event-total semantics from their own tables:
-- cvs_uploaded is the count of users holding a stored résumé (one per user, so also a
-- people count); cvs_tailored counts CVs created as a per-vacancy copy, read off the
-- is_tailored flag rather than job_id — cmd/prune nulls the link (0058); match_analyses
-- is every Analyze-match run, and since user_job_analysis is keyed (user_id, job_id)
-- that is a count of distinct candidate×vacancy matches, not of recomputes;
-- inboxes_connected adds the live Gmail grants to the claimed hosted mailboxes (both are
-- one-per-user and both are a deliberate connect action, so the sum is a count of
-- inboxes, not of people); and saved_searches is every saved search. All five read tiny
-- tables — unlike `viewed`, none of them needs a rollup to stay fast.
WITH views AS (
    -- Both view figures in one pass over the 9.9M-row rollup. `FILTER` rather than a
    -- second subquery with a WHERE: see the note above for what the second pass cost.
    SELECT COALESCE(sum(page_uniques), 0)::int            AS viewed,
           min(day) FILTER (WHERE page_uniques > 0)::date AS viewed_since
    FROM job_daily_views
)
SELECT
    count(*) FILTER (WHERE saved_at IS NOT NULL)::int   AS saved,
    -- applications live in their own table now, and counting them there is also the
    -- honest count: one that outlived its posting is still an application somebody made.
    (SELECT count(*) FROM applications WHERE applied_at IS NOT NULL)::int AS applied,
    -- Read as two scalar subqueries over the CTE, never joined to user_jobs: a
    -- CROSS JOIN would need a GROUP BY, and that turns an EMPTY user_jobs from one
    -- all-zero row into NO rows, which is an error for a `:one` query.
    (SELECT viewed FROM views)       AS viewed,
    (SELECT viewed_since FROM views) AS viewed_since,
    (SELECT count(*) FROM users WHERE resume_object_key IS NOT NULL)::int AS cvs_uploaded,
    (SELECT count(*) FROM cvs WHERE is_tailored)::int AS cvs_tailored,
    (SELECT count(*) FROM user_job_analysis)::int AS match_analyses,
    ((SELECT count(*) FROM gmail_connections WHERE status = 'connected')
        + (SELECT count(*) FROM mailboxes))::int AS inboxes_connected,
    (SELECT count(*) FROM saved_searches)::int AS saved_searches
FROM user_jobs;

-- name: ListJobActivity :many
-- Dense activity series over [from, to] at the given granularity. A daily
-- generate_series builds the gap-free calendar; the LEFT JOIN fills each day's
-- counts (missing days → 0), and date_trunc(unit, ...) rolls those days up to the
-- requested bucket (day/week/month) so empty buckets still appear as zeros. `unit`
-- is a caller-validated date_trunc field (day/week/month), never raw user input.
SELECT
    date_trunc(sqlc.arg('unit')::text, d)::date AS period,
    COALESCE(sum(s.added), 0)::int   AS added,
    COALESCE(sum(s.removed), 0)::int AS removed
FROM generate_series(sqlc.arg('from_ts')::timestamp, sqlc.arg('to_ts')::timestamp, interval '1 day') AS d
LEFT JOIN job_daily_stats s ON s.day = d::date
GROUP BY date_trunc(sqlc.arg('unit')::text, d)
ORDER BY date_trunc(sqlc.arg('unit')::text, d);
