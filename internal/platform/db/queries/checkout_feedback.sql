-- name: RecordCheckoutFeedbackSession :exec
INSERT INTO checkout_feedback_sessions (user_id, stripe_session_id, created_at)
VALUES ($1, $2, $3)
ON CONFLICT (user_id) DO UPDATE
SET stripe_session_id = EXCLUDED.stripe_session_id, created_at = EXCLUDED.created_at
WHERE checkout_feedback_sessions.created_at <= EXCLUDED.created_at;

-- name: ListCheckoutFeedbackDue :many
-- A recent consumer checkout is evidence of intent. The runner checks with Stripe
-- before claiming the letter, and the claim rechecks everything that can change here.
SELECT c.user_id, c.stripe_session_id, mail_recipient(u.email, u.email_verified, u.last_active_at, u.banned_at)::text AS email
FROM checkout_feedback_sessions c
JOIN users u ON u.id = c.user_id
LEFT JOIN notification_settings ns ON ns.user_id = c.user_id
WHERE c.user_id > sqlc.arg(after_user_id)::bigint
  AND c.created_at <= now() - interval '24 hours'
  AND c.created_at > now() - interval '72 hours'
  AND NOT cv_lifecycle_holdout(c.user_id)
  AND mail_recipient(u.email, u.email_verified, u.last_active_at, u.banned_at) <> ''
  AND COALESCE(ns.news_email_enabled, true)
  AND (u.pro_until IS NULL OR u.pro_until <= now())
  AND (u.ultra_until IS NULL OR u.ultra_until <= now())
  AND NOT EXISTS (SELECT 1 FROM cv_lifecycle_sends s WHERE s.user_id = c.user_id AND s.step = 5)
  AND NOT EXISTS (SELECT 1 FROM mail_day_claims d WHERE d.user_id = c.user_id AND d.day = mail_today())
ORDER BY c.user_id
LIMIT sqlc.arg(max_rows)::int;

-- name: ClaimCheckoutFeedbackSend :one
INSERT INTO cv_lifecycle_sends (user_id, step)
SELECT c.user_id, 5
FROM checkout_feedback_sessions c
JOIN users u ON u.id = c.user_id
LEFT JOIN notification_settings ns ON ns.user_id = c.user_id
WHERE c.user_id = sqlc.arg(user_id)
  AND c.stripe_session_id = sqlc.arg(stripe_session_id)
  AND c.created_at <= now() - interval '24 hours'
  AND c.created_at > now() - interval '72 hours'
  AND NOT cv_lifecycle_holdout(c.user_id)
  AND mail_recipient(u.email, u.email_verified, u.last_active_at, u.banned_at) = sqlc.arg(email)::text
  AND sqlc.arg(email)::text <> ''
  AND COALESCE(ns.news_email_enabled, true)
  AND (u.pro_until IS NULL OR u.pro_until <= now())
  AND (u.ultra_until IS NULL OR u.ultra_until <= now())
FOR NO KEY UPDATE OF c
ON CONFLICT (user_id, step) DO NOTHING
RETURNING user_id;

-- name: PruneCheckoutFeedbackSessions :exec
DELETE FROM checkout_feedback_sessions
WHERE created_at <= now() - interval '72 hours';
