-- name: ListInboxDigestCandidates :many
-- Filter due accounts before the limit; empty and non-due accounts cannot starve others.
SELECT u.id AS user_id,
 mail_recipient(u.email, u.email_verified, u.last_active_at, u.banned_at)::text AS email,
 inbox_digest_timezone(u.timezone)::text AS timezone,
 coalesce(ns.digest_time, '09:00'::time)::time AS digest_time,
 coalesce(CASE WHEN ns.inbox_digest_enabled THEN ns.inbox_digest_enabled_at END, u.interview_scheduling_enabled_at)::timestamptz AS enabled_at
FROM (
 SELECT user_id FROM notification_settings WHERE enabled AND inbox_digest_enabled
 UNION
 SELECT id FROM users WHERE interview_scheduling_enabled_at IS NOT NULL
) candidate
JOIN users u ON u.id = candidate.user_id
LEFT JOIN notification_settings ns ON u.id = ns.user_id
WHERE coalesce(ns.enabled, true)
 AND (coalesce(ns.inbox_digest_enabled, false) OR u.interview_scheduling_enabled_at IS NOT NULL)
 AND (coalesce(u.pro_until > greatest(sqlc.arg(now_at)::timestamptz, now()), false) OR coalesce(u.ultra_until > greatest(sqlc.arg(now_at)::timestamptz, now()), false))
 AND coalesce(ns.channels, ARRAY['email']::text[]) @> ARRAY['email']::text[]
 AND mail_recipient(u.email, u.email_verified, u.last_active_at, u.banned_at) <> ''
 AND (sqlc.arg(now_at)::timestamptz AT TIME ZONE inbox_digest_timezone(u.timezone))::time >= coalesce(ns.digest_time, '09:00'::time)
 AND NOT EXISTS (
  SELECT 1 FROM inbox_digest_deliveries d WHERE d.user_id = u.id
   AND (d.local_day = (sqlc.arg(now_at)::timestamptz AT TIME ZONE inbox_digest_timezone(u.timezone))::date
    OR (d.sent_at AT TIME ZONE inbox_digest_timezone(u.timezone))::date >= (sqlc.arg(now_at)::timestamptz AT TIME ZONE inbox_digest_timezone(u.timezone))::date)
 )
 AND (EXISTS (
  SELECT 1 FROM emails e
  WHERE coalesce(ns.inbox_digest_enabled, false) AND e.user_id = u.id AND e.direction = 'incoming' AND e.deleted_at IS NULL
   AND e.classified_at IS NOT NULL AND e.status_signal <> 'other'
   -- A late classification must not revive a request superseded by a delivered reply.
   AND NOT EXISTS (
    SELECT 1 FROM emails newer
    WHERE e.thread_id <> '' AND newer.user_id = e.user_id AND newer.source = e.source
     AND newer.thread_id = e.thread_id AND newer.direction = 'incoming'
     AND (newer.received_at, newer.id) > (e.received_at, e.id)
     AND EXISTS (SELECT 1 FROM inbox_digest_deliveries d WHERE d.user_id = newer.user_id AND d.sent_at IS NOT NULL AND newer.id = ANY(d.email_ids))
   )
   AND e.received_at >= greatest(ns.inbox_digest_enabled_at, sqlc.arg(now_at)::timestamptz - interval '7 days')
   AND e.received_at <= sqlc.arg(now_at)::timestamptz
   AND NOT EXISTS (SELECT 1 FROM inbox_digest_deliveries d WHERE d.user_id = u.id AND d.sent_at IS NOT NULL AND e.id = ANY(d.email_ids))
 ) OR EXISTS (
  SELECT 1 FROM interview_scheduling_activity activity
  WHERE activity.user_id = u.id AND u.interview_scheduling_enabled_at IS NOT NULL
    AND activity.happened_at >= greatest(u.interview_scheduling_enabled_at, sqlc.arg(now_at)::timestamptz - interval '7 days')
    AND activity.happened_at <= sqlc.arg(now_at)::timestamptz
    AND NOT EXISTS (SELECT 1 FROM inbox_digest_deliveries d WHERE d.user_id = u.id AND d.sent_at IS NOT NULL
                    AND activity.id = ANY(d.scheduling_activity_ids))
 ))
ORDER BY u.id
LIMIT sqlc.arg(batch_size);

-- name: GetInboxDigestSnapshot :one
-- Retire the entire covered window on success, including overflow. Display only the newest
-- message in each provider conversation; messages without a thread remain separate.
WITH eligible AS MATERIALIZED (
 SELECT e.*,
  CASE WHEN e.thread_id <> '' THEN 'thread:' || e.thread_id ELSE 'message:' || e.id::text END AS conversation
 FROM emails e
 WHERE e.user_id = sqlc.arg(user_id) AND e.direction = 'incoming' AND e.deleted_at IS NULL
  AND e.classified_at IS NOT NULL AND e.status_signal <> 'other'
   -- A late classification must not revive a request superseded by a delivered reply.
   AND NOT EXISTS (
    SELECT 1 FROM emails newer
    WHERE e.thread_id <> '' AND newer.user_id = e.user_id AND newer.source = e.source
     AND newer.thread_id = e.thread_id AND newer.direction = 'incoming'
     AND (newer.received_at, newer.id) > (e.received_at, e.id)
     AND EXISTS (SELECT 1 FROM inbox_digest_deliveries d WHERE d.user_id = newer.user_id AND d.sent_at IS NOT NULL AND newer.id = ANY(d.email_ids))
   )
  AND EXISTS (SELECT 1 FROM notification_settings ns WHERE ns.user_id = e.user_id AND ns.inbox_digest_enabled
              AND e.received_at >= ns.inbox_digest_enabled_at)
  AND e.received_at >= greatest(sqlc.arg(enabled_at)::timestamptz, sqlc.arg(now_at)::timestamptz - interval '7 days')
  AND e.received_at <= sqlc.arg(now_at)::timestamptz
  AND NOT EXISTS (SELECT 1 FROM inbox_digest_deliveries d WHERE d.user_id = e.user_id AND d.sent_at IS NOT NULL AND e.id = ANY(d.email_ids))
), conversations AS (
 SELECT DISTINCT ON (source, conversation) * FROM eligible
 ORDER BY source, conversation, received_at DESC, id DESC
), shown AS (
 SELECT * FROM conversations ORDER BY received_at DESC, id DESC LIMIT 10
), activity AS MATERIALIZED (
 SELECT a.id, a.kind, a.happened_at, s.anchor_email_id, e.subject
 FROM interview_scheduling_activity a
 JOIN interview_scheduling_threads s ON s.id = a.thread_id
 JOIN emails e ON e.id = s.anchor_email_id
 JOIN users u ON u.id = a.user_id
 WHERE a.user_id = sqlc.arg(user_id) AND u.interview_scheduling_enabled_at IS NOT NULL
   AND a.happened_at >= greatest(u.interview_scheduling_enabled_at, sqlc.arg(now_at)::timestamptz - interval '7 days')
   AND a.happened_at <= sqlc.arg(now_at)::timestamptz
   AND NOT EXISTS (SELECT 1 FROM inbox_digest_deliveries d WHERE d.user_id = a.user_id AND d.sent_at IS NOT NULL
                   AND a.id = ANY(d.scheduling_activity_ids))
), activity_shown AS (
 SELECT * FROM activity ORDER BY happened_at DESC, id DESC LIMIT 10
)
SELECT
 coalesce((SELECT array_agg(id ORDER BY id) FROM eligible), '{}')::bigint[] AS email_ids,
 (SELECT count(*) FROM conversations)::bigint AS total,
 coalesce((SELECT array_agg(id ORDER BY id) FROM activity), '{}')::bigint[] AS scheduling_activity_ids,
 coalesce((SELECT jsonb_agg(jsonb_build_object('id', anchor_email_id, 'subject', subject, 'kind', kind)
                           ORDER BY happened_at DESC, id DESC) FROM activity_shown), '[]'::jsonb)::jsonb AS scheduling_activity,
 coalesce((SELECT jsonb_agg(jsonb_build_object(
  'id', s.id, 'subject', s.subject, 'sender', coalesce(nullif(s.from_name,''), s.from_addr),
  'snippet', left(regexp_replace(s.body_text, E'\\s+', ' ', 'g'), 240),
  'signal', s.status_signal, 'received_at', s.received_at,
  'replied', EXISTS (SELECT 1 FROM emails reply WHERE reply.user_id = s.user_id AND reply.source = s.source
   AND s.thread_id <> '' AND reply.thread_id = s.thread_id AND reply.direction = 'outgoing'
   AND reply.deleted_at IS NULL AND reply.received_at > s.received_at AND reply.received_at <= sqlc.arg(now_at)::timestamptz)
 ) ORDER BY s.received_at DESC, s.id DESC) FROM shown s), '[]'::jsonb)::jsonb AS messages;

-- name: ClaimInboxDigest :execrows
-- Recheck live consent and recipient eligibility immediately before reserving the day.
INSERT INTO inbox_digest_deliveries(user_id, local_day, email_ids, scheduling_activity_ids, claimed_at)
SELECT u.id, sqlc.arg(local_day)::date, coalesce(sqlc.arg(email_ids)::bigint[], '{}'), coalesce(sqlc.arg(scheduling_activity_ids)::bigint[], '{}'),
       sqlc.arg(now_at)::timestamptz
FROM users u LEFT JOIN notification_settings ns ON u.id = ns.user_id
WHERE u.id = sqlc.arg(user_id) AND coalesce(ns.enabled, true)
 AND (coalesce(ns.inbox_digest_enabled, false) OR u.interview_scheduling_enabled_at IS NOT NULL)
 AND (coalesce(u.pro_until > greatest(sqlc.arg(now_at)::timestamptz, now()), false) OR coalesce(u.ultra_until > greatest(sqlc.arg(now_at)::timestamptz, now()), false))
 AND coalesce(ns.channels, ARRAY['email']::text[]) @> ARRAY['email']::text[]
 AND mail_recipient(u.email, u.email_verified, u.last_active_at, u.banned_at) = sqlc.arg(email)::text
 AND sqlc.arg(email)::text <> ''
 AND inbox_digest_timezone(u.timezone) = sqlc.arg(timezone)::text
 AND (sqlc.arg(now_at)::timestamptz AT TIME ZONE inbox_digest_timezone(u.timezone))::date = sqlc.arg(local_day)::date
 AND (sqlc.arg(now_at)::timestamptz AT TIME ZONE inbox_digest_timezone(u.timezone))::time >= coalesce(ns.digest_time, '09:00'::time) AND (cardinality(sqlc.arg(email_ids)::bigint[]) > 0 OR cardinality(sqlc.arg(scheduling_activity_ids)::bigint[]) > 0)
 AND coalesce(CASE WHEN ns.inbox_digest_enabled THEN ns.inbox_digest_enabled_at END, u.interview_scheduling_enabled_at) = sqlc.arg(enabled_at)::timestamptz
ON CONFLICT (user_id, local_day) DO NOTHING;

-- name: MarkInboxDigestSent :execrows
UPDATE inbox_digest_deliveries SET sent_at = sqlc.arg(sent_at)::timestamptz
WHERE user_id = sqlc.arg(user_id) AND local_day = sqlc.arg(local_day)::date AND sent_at IS NULL;

-- name: ReleasePausedInboxDigest :exec
-- Used only when dispatch was definitely withheld (e.g. the SES circuit breaker).
DELETE FROM inbox_digest_deliveries
WHERE user_id = sqlc.arg(user_id) AND local_day = sqlc.arg(local_day)::date AND sent_at IS NULL;
