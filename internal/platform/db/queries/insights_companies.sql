-- Per-company Trends & Insights rollups (insights_company_*) and their reads, split out of
-- insights.sql for size. Its header states the rules every rollup here keeps too: a prior-
-- window count asks whether a posting EXISTED at @prev_ts (from LEAST(posted_at,
-- created_at)), and the rule tests in this package read every queries/insights*.sql file.

-- ---------------------------------------------------------------------------
-- Per-company hiring signal
-- ---------------------------------------------------------------------------

-- name: ComputeInsightsCompanyStats :many
-- Compute on the read connection; publish the result separately on the writer.
-- Per-(company, day) hiring velocity with a running open count, from the retained
-- jobs lifecycle. Each canonical, attributable job (company_slug <> '' AND
-- duplicate_of IS NULL) emits an added event on its created_at (UTC) day and, if
-- closed, a removed event on its closed_at (UTC) day; the inner aggregate collapses
-- those to one row per (company_slug, day), and the window SUM over (added - removed)
-- ordered by day yields open = cumulative(added) - cumulative(removed) as of that day
-- (a job is created no later than it closes, so this equals the point-in-time open
-- count). Only a company's activity days get a row.
-- Per-company open/growth scalar (backs the /insights/companies leaderboard)
SELECT
    (company_slug)::text AS company_slug,
    (day)::date AS day,
    (added)::integer AS added,
    (removed)::integer AS removed,
    (sum(added - removed) OVER (PARTITION BY company_slug ORDER BY day)::int)::integer AS open
FROM (
    SELECT j.company_slug, day, sum(added)::int AS added, sum(removed)::int AS removed
    FROM jobs j
    CROSS JOIN LATERAL (

        SELECT (j.created_at AT TIME ZONE 'UTC')::date AS day, 1 AS added, 0 AS removed
        UNION ALL
        SELECT (j.closed_at AT TIME ZONE 'UTC')::date, 0, 1
        WHERE j.closed_at IS NOT NULL
    ) e
    WHERE j.company_slug <> '' AND j.duplicate_of IS NULL
    GROUP BY j.company_slug, day
) daily;

-- name: DeleteAllInsightsCompanyGrowth :exec
DELETE FROM insights_company_growth;

-- name: ComputeInsightsCompanyGrowth :many
-- Compute on the read connection; publish the result separately on the writer.
-- One row per company with its current open-count and the open-count as of @prev_ts,
-- from a single scan of jobs over canonical rows only (same count(*) FILTER idiom as
-- insights_role_stats). open_count uses closed_at IS NULL (open now); open_count_prev
-- uses open-as-of @prev_ts. Companies open at neither point are dropped (HAVING) to
-- keep the table lean.
SELECT
    (company_slug)::text AS company_slug,
    (count(*) FILTER (WHERE closed_at IS NULL)::int)::integer AS open_count,
    (count(*) FILTER (WHERE LEAST(posted_at, created_at) <= sqlc.arg('prev_ts') AND (closed_at IS NULL OR closed_at > sqlc.arg('prev_ts')))::int)::integer AS open_count_prev
FROM jobs
WHERE company_slug <> '' AND duplicate_of IS NULL
GROUP BY company_slug
HAVING count(*) FILTER (WHERE closed_at IS NULL) > 0
    OR count(*) FILTER (WHERE LEAST(posted_at, created_at) <= sqlc.arg('prev_ts') AND (closed_at IS NULL OR closed_at > sqlc.arg('prev_ts'))) > 0;

-- name: ListInsightsCompanies :many
-- The leaderboard read: companies ranked by growth (open_count - open_count_prev),
-- '-growth' (freezing) reverses it, 'open' ranks by raw size; open_count is the
-- tiebreak. @min_open floors the current open-count (blunts ingest-artifact spikes).
-- company_name falls back to the slug when no companies row exists.
SELECT
    g.company_slug,
    coalesce(c.name, g.company_slug) AS company_name,
    g.open_count,
    g.open_count_prev,
    (g.open_count - g.open_count_prev)::int AS growth
FROM insights_company_growth g
LEFT JOIN companies c ON c.slug = g.company_slug
WHERE g.open_count >= sqlc.arg('min_open')::int
ORDER BY
    (CASE
        WHEN sqlc.arg('sort')::text = 'open'     THEN g.open_count
        WHEN sqlc.arg('sort')::text = '-growth'  THEN -(g.open_count - g.open_count_prev)
        ELSE (g.open_count - g.open_count_prev)
    END) DESC,
    g.open_count DESC
LIMIT sqlc.arg('lim')::int;

-- name: DeleteAllInsightsCompanyResponse :exec
DELETE FROM insights_company_response;

-- name: ComputeInsightsCompanyResponse :many
-- Compute on the read connection; publish the result separately on the writer.
-- One row per company with an OBSERVABLE application: the applicant has a connected
-- mailbox, so a reply would have been seen. Applications from people we cannot observe
-- are excluded from BOTH sides of the ratio — counting them in the denominator would
-- report our own blind spot as the employer's silence, which is the same mistake the
-- job-level signal's mailbox gate exists to prevent.
-- Both sides come from application_events, not from live mail. Reading the emails table
-- made the served rate a function of the candidate's inbox hygiene: it counted messages
-- WHERE deleted_at IS NULL, so someone clearing old mail made an employer that had
-- answered them look silent, on a public page, about a named company. The ledger records
-- that the reply arrived; what later became of the message is not the employer's doing.
-- "Answered" is any live employer_reply event. Not a stage advance: a stage is what the
-- candidate recorded, and a company that replied to somebody who never updated their
-- board still replied. A retracted event does not count — it belongs to another employer.
-- The reply is paired to its application through application_id, never through job_id.
-- job_id is ON DELETE SET NULL, so once a posting is deleted both sides read NULL, the pair reduces
-- to NULL = NULL — never true — and an employer that answered is served as silent: the
-- distortion this rollup was moved onto the ledger to remove, arriving by a different
-- route. Pairing on (user_id, company_slug) instead would survive the deletion by
-- crediting one reply to every application that user made to that employer, which is
-- worse than losing it.
-- An event with no application_id is left out of BOTH sides rather than counted and never
-- answerable. Only rows written before the carry-over can be in that state, and
-- cmd/backfill-applications is what empties it.
-- The median covers answered applications only and is therefore right-censored; the
-- serving layer reports the unanswered count beside it so the number is never read as
-- "employers reply in six days" when most of the sample was never replied to. Replies
-- dated before their application (an application reconstructed from a later message, or
-- clock skew) still count as answered but are kept out of the median, where they would
-- contribute a negative duration.
WITH observable AS (
    SELECT ae.application_id, ae.company_slug, ae.occurred_at AS applied_at
      FROM application_events ae
     WHERE ae.kind = 'applied'
       AND ae.retracted_at IS NULL
       AND ae.company_slug <> ''
       AND ae.application_id IS NOT NULL






       AND (EXISTS (SELECT 1 FROM gmail_connections gc
                     WHERE gc.user_id = ae.user_id AND gc.status = 'connected' AND gc.email <> '')
         OR EXISTS (SELECT 1 FROM mailboxes mb WHERE mb.user_id = ae.user_id))
), answered AS (
    SELECT o.company_slug, o.applied_at,
           (SELECT min(r.occurred_at)
              FROM application_events r
             WHERE r.application_id = o.application_id
               AND r.kind          = 'employer_reply'
               AND r.retracted_at IS NULL) AS replied_at
      FROM observable o
)

SELECT
    (company_slug)::text AS company_slug,
    (count(*)::int)::integer AS applications,
    (count(*) FILTER (WHERE replied_at IS NOT NULL)::int)::integer AS answered,
    coalesce((percentile_cont(0.5) WITHIN GROUP (
           ORDER BY EXTRACT(EPOCH FROM (replied_at - applied_at)) / 86400.0
       ) FILTER (WHERE replied_at IS NOT NULL AND replied_at >= applied_at)::real)::real, 0::real)::real AS median_reply_days,
    ((percentile_cont(0.5) WITHIN GROUP (
           ORDER BY EXTRACT(EPOCH FROM (replied_at - applied_at)) / 86400.0
       ) FILTER (WHERE replied_at IS NOT NULL AND replied_at >= applied_at)::real) IS NOT NULL)::boolean AS median_reply_days_valid
FROM answered
 GROUP BY company_slug;

-- name: GetCompanyResponse :one
-- The observable application counts for one company. A company with no row has no
-- observable applications at all, which the caller must treat as "not enough data"
-- rather than as a zero response rate.
SELECT applications, answered, median_reply_days FROM insights_company_response WHERE company_slug = $1;

-- ---------------------------------------------------------------------------
-- Global and per-user application response rate (the reply-rate benchmark)
-- ---------------------------------------------------------------------------

-- name: GetGlobalCompanyResponse :one
-- The all-companies response rate, summed from the same per-company observable and
-- answered counts ComputeInsightsCompanyResponse already computed. Every company's row
-- contributes regardless of whether it individually clears its own ten-application
-- sample gate (see company-hiring-signal) — that gate protects what is safe to publish
-- about one NAMED employer, not what may contribute to an aggregate across all of them.
-- The caller applies its own sample gate to this sum.
SELECT
    coalesce(sum(applications), 0)::int AS applications,
    coalesce(sum(answered), 0)::int     AS answered
  FROM insights_company_response;

-- name: GetUserResponseRate :one
-- One caller's own observable/answered counts, computed live from application_events —
-- unlike the per-company/global figures, this is not read from a periodic rollup, since
-- it is scoped to one user and cheap to compute on every request (served by
-- application_events_user_occurred_idx). Same observable/answered definitions as
-- ComputeInsightsCompanyResponse: observable requires a connected mailbox, answered
-- requires a non-retracted employer_reply event. A caller with no connected mailbox has
-- zero observable applications — not a separate case, just the same predicate producing
-- zero — which is what lets the serving layer's sample gate double as the "no mailbox"
-- check.
-- No applied_at here, unlike the per-company CTE this mirrors: that one needs it for the
-- median days-to-reply, and the personal side serves no time-based metric (see design.md
-- - Non-Goals).
WITH observable AS (
    SELECT ae.application_id
      FROM application_events ae
     WHERE ae.kind = 'applied'
       AND ae.retracted_at IS NULL
       AND ae.user_id = $1
       AND ae.company_slug <> ''
       AND ae.application_id IS NOT NULL
       AND (EXISTS (SELECT 1 FROM gmail_connections gc
                     WHERE gc.user_id = ae.user_id AND gc.status = 'connected' AND gc.email <> '')
         OR EXISTS (SELECT 1 FROM mailboxes mb WHERE mb.user_id = ae.user_id))
)
SELECT
    count(*)::int AS applications,
    count(*) FILTER (
        WHERE EXISTS (SELECT 1 FROM application_events r
                       WHERE r.application_id = o.application_id
                         AND r.kind          = 'employer_reply'
                         AND r.retracted_at IS NULL)
    )::int AS answered
  FROM observable o;

-- ---------------------------------------------------------------------------
-- Per-company posting lifetime (backs the company page's Hiring tab)
-- ---------------------------------------------------------------------------

-- name: DeleteAllInsightsCompanyLifetime :exec
DELETE FROM insights_company_lifetime;

-- name: ComputeInsightsCompanyLifetime :many
-- Compute on the read connection; publish the result separately on the writer.
-- Median days from created_at to closed_at over each company's canonical postings
-- closed in the last 180 days. Only closures that say the POSTING is over count: it
-- dropped off a board we crawl (unseen), the source reported it taken down
-- (feed_removed), the liveness probe read its source page as expired twice
-- (probe_expired), or the employer closed it here (employer_closed). Not expired —
-- that is the age rule for sources with no close signal, which closes at 45 days
-- whatever the employer did, so it would publish our cutoff as the company's speed.
-- board_unreachable and feed_empty measure our crawler, board_retired,
-- source_misattributed and moderated our own decisions, '' is unknown. 180 days so the
-- figure describes how the company hires now, not over its whole history.
-- Every rollup that measures a real close reads this same list;
-- insights_close_rule_test.go fails if one of them drifts from it.
SELECT
    (company_slug)::text AS company_slug,
    (count(*)::int)::integer AS closed,
    (percentile_cont(0.5) WITHIN GROUP (
        ORDER BY EXTRACT(EPOCH FROM (closed_at - created_at)) / 86400.0
    )::real)::real AS median_days
FROM jobs
WHERE company_slug <> ''
  AND duplicate_of IS NULL



  AND NOT is_private
  AND closed_at IS NOT NULL
  AND closed_at >= sqlc.arg('as_of')::timestamptz - interval '180 days'
  AND closed_reason IN ('unseen', 'feed_removed', 'probe_expired', 'employer_closed')


  AND closed_at >= created_at
GROUP BY company_slug;

-- name: GetCompanyLifetime :one
-- One company's posting lifetime. No row means no qualifying closure; the caller
-- applies its own sample gate on `closed`.
SELECT closed, median_days FROM insights_company_lifetime WHERE company_slug = $1;

-- name: GetCompanyGrowth :one
-- One company's open count now and as of the growth window ago. No row means nothing
-- open at either point; the caller serves zeros.
SELECT open_count, open_count_prev FROM insights_company_growth WHERE company_slug = $1;

-- name: ListCompanyActivity :many
-- One company's dense added/removed series over [from, to] at the given granularity —
-- the company-scoped twin of ListJobActivity. insights_company_stats holds rows only on
-- a company's activity days, so a daily generate_series builds the gap-free calendar,
-- the LEFT JOIN on the (company_slug, day) primary key fills it (missing days → 0), and
-- date_trunc rolls days up to the bucket. `unit` is caller-validated (day/week/month).
SELECT
    date_trunc(sqlc.arg('unit')::text, d)::date AS period,
    COALESCE(sum(s.added), 0)::int   AS added,
    COALESCE(sum(s.removed), 0)::int AS removed
FROM generate_series(sqlc.arg('from_ts')::timestamp, sqlc.arg('to_ts')::timestamp, interval '1 day') AS d
LEFT JOIN insights_company_stats s
       ON s.company_slug = sqlc.arg('company_slug')::text AND s.day = d::date
GROUP BY date_trunc(sqlc.arg('unit')::text, d)
ORDER BY date_trunc(sqlc.arg('unit')::text, d);

-- ---------------------------------------------------------------------------
-- Per-company posting views (backs the company page's Hiring tab)
-- ---------------------------------------------------------------------------

-- name: DeleteAllInsightsCompanyViews :exec
DELETE FROM insights_company_views;

-- name: ComputeInsightsCompanyViews :many
-- Compute on the read connection; publish the result separately on the writer.
-- page_uniques summed over each company's postings for the last 30 days and the 30
-- before. page_uniques, never uniques: uniques fuses unfiltered API reads — mostly
-- crawlers — with page opens. Duplicates are NOT filtered out: a view of a repost is
-- still attention paid to the company. One visitor counts once per posting per day, so
-- the sum is posting-views, not distinct people; the page says so.
SELECT
    (j.company_slug)::text AS company_slug,
    (COALESCE(sum(v.page_uniques) FILTER (WHERE v.day > current_date - 30), 0)::bigint)::bigint AS views,
    (COALESCE(sum(v.page_uniques) FILTER (WHERE v.day <= current_date - 30), 0)::bigint)::bigint AS views_prev
FROM job_daily_views v
JOIN jobs j ON j.id = v.job_id
WHERE v.day > current_date - 60
  AND j.company_slug <> ''
GROUP BY j.company_slug
HAVING sum(v.page_uniques) > 0;

-- name: GetCompanyViews :one
-- One company's posting views. No row means none in either window; the caller serves zeros.
SELECT views, views_prev FROM insights_company_views WHERE company_slug = $1;
