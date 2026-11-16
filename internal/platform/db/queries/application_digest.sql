-- name: ListApplicationDigestCandidates :many
-- Keyset pagination lets the worker skip empty snapshots without starving later accounts.
SELECT ns.user_id,
 mail_recipient(u.email, u.email_verified, u.last_active_at, u.banned_at)::text AS email,
 inbox_digest_timezone(u.timezone)::text AS timezone,
 coalesce(ns.digest_time, '09:00'::time)::time AS digest_time,
 ns.application_digest_enabled_at AS enabled_at
FROM notification_settings ns
JOIN users u ON u.id = ns.user_id
WHERE ns.user_id > sqlc.arg(after_user_id) AND ns.enabled AND ns.application_digest_enabled
 AND (coalesce(u.pro_until > greatest(sqlc.arg(now_at)::timestamptz, now()), false) OR coalesce(u.ultra_until > greatest(sqlc.arg(now_at)::timestamptz, now()), false))
 AND ns.channels @> ARRAY['email']::text[]
 AND mail_recipient(u.email, u.email_verified, u.last_active_at, u.banned_at) <> ''
 AND (sqlc.arg(now_at)::timestamptz AT TIME ZONE inbox_digest_timezone(u.timezone))::time >= coalesce(ns.digest_time, '09:00'::time)
 AND extract(isodow FROM sqlc.arg(now_at)::timestamptz AT TIME ZONE inbox_digest_timezone(u.timezone)) = 1
 AND NOT EXISTS (
  SELECT 1 FROM application_digest_deliveries d WHERE d.user_id = ns.user_id
   AND (d.local_week = (sqlc.arg(now_at)::timestamptz AT TIME ZONE inbox_digest_timezone(u.timezone))::date
    OR (d.sent_at AT TIME ZONE inbox_digest_timezone(u.timezone))::date >= (sqlc.arg(now_at)::timestamptz AT TIME ZONE inbox_digest_timezone(u.timezone))::date)
 )
ORDER BY ns.user_id
LIMIT sqlc.arg(batch_size);

-- name: ClaimApplicationDigest :execrows
-- Recheck live consent and recipient eligibility immediately before reserving the week.
INSERT INTO application_digest_deliveries(user_id, local_week, claimed_at)
SELECT ns.user_id, sqlc.arg(local_week)::date, sqlc.arg(now_at)::timestamptz
FROM notification_settings ns JOIN users u ON u.id = ns.user_id
WHERE ns.user_id = sqlc.arg(user_id) AND ns.enabled AND ns.application_digest_enabled
 AND (coalesce(u.pro_until > greatest(sqlc.arg(now_at)::timestamptz, now()), false) OR coalesce(u.ultra_until > greatest(sqlc.arg(now_at)::timestamptz, now()), false))
 AND ns.channels @> ARRAY['email']::text[]
 AND mail_recipient(u.email, u.email_verified, u.last_active_at, u.banned_at) = sqlc.arg(email)::text
 AND sqlc.arg(email)::text <> ''
 AND inbox_digest_timezone(u.timezone) = sqlc.arg(timezone)::text
 AND (sqlc.arg(now_at)::timestamptz AT TIME ZONE inbox_digest_timezone(u.timezone))::date = sqlc.arg(local_week)::date
 AND (sqlc.arg(now_at)::timestamptz AT TIME ZONE inbox_digest_timezone(u.timezone))::time >= coalesce(ns.digest_time, '09:00'::time)
 AND extract(isodow FROM sqlc.arg(now_at)::timestamptz AT TIME ZONE inbox_digest_timezone(u.timezone)) = 1
 AND NOT EXISTS (SELECT 1 FROM application_digest_deliveries d WHERE d.user_id = ns.user_id AND (d.sent_at AT TIME ZONE inbox_digest_timezone(u.timezone))::date >= sqlc.arg(local_week)::date)
 AND ns.application_digest_enabled_at = sqlc.arg(enabled_at)::timestamptz
ON CONFLICT (user_id, local_week) DO NOTHING;

-- name: MarkApplicationDigestSent :execrows
UPDATE application_digest_deliveries SET sent_at = sqlc.arg(sent_at)::timestamptz
WHERE user_id = sqlc.arg(user_id) AND local_week = sqlc.arg(local_week)::date AND sent_at IS NULL;

-- name: ReleasePausedApplicationDigest :exec
-- Used only when dispatch was definitely withheld (e.g. the SES circuit breaker).
DELETE FROM application_digest_deliveries
WHERE user_id = sqlc.arg(user_id) AND local_week = sqlc.arg(local_week)::date AND sent_at IS NULL;

-- name: GetApplicationDigestSnapshot :one
-- Live, unretracted ledger facts; the current board is deliberately a separate view.
WITH events AS (
 SELECT ev.* FROM application_events ev
 WHERE ev.user_id = sqlc.arg(user_id) AND retracted_at IS NULL
 AND occurred_at >= sqlc.arg(now_at)::timestamptz - interval '7 days'
 AND occurred_at <= sqlc.arg(now_at)::timestamptz
), stages AS (
 SELECT coalesce(stage, 'applied')::text AS stage, count(*)::bigint AS count
 FROM applications a WHERE a.user_id = sqlc.arg(user_id) AND (stage IS NOT NULL OR applied_at IS NOT NULL)
 GROUP BY coalesce(stage, 'applied')
)
SELECT
 (SELECT count(*) FROM events)::bigint AS activity_count,
 (SELECT count(*) FROM events WHERE kind = 'applied')::bigint AS applied,
 (SELECT count(*) FROM events WHERE kind = 'employer_reply')::bigint AS replies,
 (SELECT count(*) FROM events WHERE kind = 'stage_set' AND signal = 'interview')::bigint AS interviews,
 (SELECT count(*) FROM events WHERE kind = 'stage_set' AND signal = 'offer')::bigint AS offers,
 coalesce((SELECT jsonb_agg(jsonb_build_object('stage', stage, 'count', count) ORDER BY stage) FROM stages), '[]'::jsonb)::jsonb AS stages;

-- name: ListApplicationDigestSilentCandidates :many
-- Mirror the board's activity clock: a candidate's chase or stage edit is not a reply.
SELECT a.id, a.job_id, coalesce(j.public_slug, '')::text AS job_slug,
 a.company_slug, a.role_title, coalesce(a.stage, '')::text AS stage,
 a.applied_at, a.followed_up_at,
 greatest(a.applied_at, mail.newest_mail_at, emp.newest_move_at)::timestamptz AS last_activity_at,
 coalesce(mail.suggestion_pending, false)::boolean AS has_pending_suggestion
FROM applications a
LEFT JOIN jobs j ON j.id = a.job_id
LEFT JOIN LATERAL (
 SELECT max(e.received_at) FILTER (WHERE e.application_id = a.id OR e.job_id = a.job_id) AS newest_mail_at,
 bool_or(e.suggested_job_id = a.job_id AND e.application_id IS NULL) AS suggestion_pending
 FROM emails e WHERE e.user_id = a.user_id AND e.deleted_at IS NULL AND e.direction = 'incoming'
 AND (e.application_id = a.id OR e.job_id = a.job_id OR e.suggested_job_id = a.job_id)
 AND e.received_at <= sqlc.arg(now_at)::timestamptz
) mail ON true
LEFT JOIN LATERAL (
 SELECT max(ev.occurred_at) AS newest_move_at FROM application_events ev
 WHERE ev.application_id = a.id AND ev.kind = 'stage_set' AND ev.source = 'employer'
 AND ev.retracted_at IS NULL AND ev.occurred_at <= sqlc.arg(now_at)::timestamptz
) emp ON true
WHERE a.user_id = sqlc.arg(user_id) AND a.applied_at IS NOT NULL
 AND (a.stage IS NULL OR a.stage IN ('preparing', 'applied', 'screening', 'responded', 'interview', 'offer'))
ORDER BY last_activity_at, a.id;
