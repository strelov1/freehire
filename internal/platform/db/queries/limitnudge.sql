-- Queries for the limit-nudge feature (internal/engage/limitnudge): one personal mail,
-- sent once per account, to a free-tier user who has run into a plan ceiling.

-- name: RecordPlanLimitHit :exec
-- Records that a real plan-ceiling refusal happened for (user, feature, day) — any
-- tier, see migration 0178 — deduped by the primary key, so a retried request that
-- hits the same wall again the same day writes nothing further. Called from
-- plan.Store.Consume on every real refusal (not FairUse, not Shadowed); see that
-- file for the distinction.
INSERT INTO plan_limit_hits (user_id, feature, day)
VALUES (sqlc.arg(user_id), sqlc.arg(feature)::text, sqlc.arg(day))
ON CONFLICT (user_id, feature, day) DO NOTHING;

-- name: ListLimitHitUsersMissingNudgeEmail :many
-- Free-tier accounts that hit a plan ceiling at least once in the last window_days,
-- verified, opted in to news mail, and not yet sent the one-time nudge.
--
-- Driven from plan_limit_hits rather than from users: the hits table only ever holds
-- free-tier refusals, so it is the selective side of the join — reading it first and
-- joining out to users is a handful of rows, where starting from users and testing
-- each one against it would be a sequential scan of the whole table.
--
-- The tier check mirrors plan.TierOf's own rule directly rather than calling it: free
-- is "neither until reaches past now", and a NULL until reads the same as a past one.
SELECT u.id, u.email
FROM (
    SELECT DISTINCT user_id
    FROM plan_limit_hits
    WHERE day > current_date - make_interval(days => sqlc.arg(window_days)::int)
) h
JOIN users u ON u.id = h.user_id
LEFT JOIN notification_settings ns ON ns.user_id = u.id
WHERE u.email_verified
  AND u.limit_nudge_sent_at IS NULL
  AND (u.pro_until IS NULL OR u.pro_until <= now())
  AND (u.ultra_until IS NULL OR u.ultra_until <= now())
  AND COALESCE(ns.news_email_enabled, true)
ORDER BY u.id
LIMIT sqlc.arg(max_rows)::int;

-- name: SetLimitNudgeSent :execrows
-- Claims the nudge send for one account. Guarded by IS NULL so a concurrent or
-- repeated run never re-sends; 0 rows affected means somebody already claimed it.
UPDATE users
SET limit_nudge_sent_at = now()
WHERE id = sqlc.arg(id)
  AND limit_nudge_sent_at IS NULL;
