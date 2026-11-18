-- name: AddAPIUsage :exec
-- Adds one flush of buffered counts: parallel arrays, one element per (account, day).
-- Additive, so two API processes (blue and green during a flip) flushing the same day
-- both land.
INSERT INTO api_usage_daily (user_id, day, requests, errors)
SELECT u.user_id, u.day, u.requests, u.errors
FROM (
    -- Set-returning functions in one select list advance together (PostgreSQL 10+), so the
    -- arrays zip element by element.
    SELECT unnest(sqlc.arg(user_ids)::bigint[]) AS user_id,
           unnest(sqlc.arg(days)::date[]) AS day,
           unnest(sqlc.arg(requests)::bigint[]) AS requests,
           unnest(sqlc.arg(errors)::bigint[]) AS errors
) AS u
-- An account deleted between the request and the flush has nothing left to count against.
WHERE EXISTS (SELECT 1 FROM users WHERE users.id = u.user_id)
ON CONFLICT (user_id, day) DO UPDATE
SET requests = api_usage_daily.requests + EXCLUDED.requests,
    errors = api_usage_daily.errors + EXCLUDED.errors;

-- name: ListAPIUsage :many
-- The account's daily counts from `since` (a UTC date) on, oldest first. Days with no
-- requests have no row.
SELECT day, requests, errors
FROM api_usage_daily
WHERE user_id = sqlc.arg(user_id) AND day >= sqlc.arg(since)::date
ORDER BY day;
