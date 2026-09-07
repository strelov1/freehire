-- name: RecordSiteStatusSample :exec
-- Upserts today's worst-severity-so-far. GREATEST against the stored value so a
-- later good sample can never erase an earlier bad one recorded the same day.
--
-- "Today" is (now() AT TIME ZONE 'utc')::date, not a bare CURRENT_DATE: the rest
-- of this feature buckets by a UTC calendar day (Go's time.Now().UTC(), the
-- frontend's UTC-anchored history strip), and a bare CURRENT_DATE follows the
-- session's timezone instead — the same divergence social_digest.sql already
-- documents and avoids for the same reason.
INSERT INTO site_status_daily (day, worst_severity)
VALUES ((now() AT TIME ZONE 'utc')::date, $1)
ON CONFLICT (day) DO UPDATE SET
    worst_severity = GREATEST(site_status_daily.worst_severity, EXCLUDED.worst_severity),
    updated_at     = now();

-- name: SiteStatusHistory :many
-- The trailing 90 UTC calendar days of recorded daily status (today and the 89
-- days before it), oldest first. A day with no row (never sampled) is simply
-- absent — the caller must not treat that as "operational". The `>` against a
-- 90-day interval (rather than `>=` against 89) reads the same as "trailing 90
-- days" everywhere else this feature says it.
SELECT day, worst_severity
FROM site_status_daily
WHERE day > (now() AT TIME ZONE 'utc')::date - INTERVAL '90 days'
ORDER BY day;
