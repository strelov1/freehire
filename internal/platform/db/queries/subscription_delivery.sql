-- name: GetSubscriptionForDelivery :one
-- The delivery context for one subscription: channel + destination, the saved
-- search id (which subscriptions of the same search share, and the notification
-- centre's dedup key is built on — see notify.digestDedupKey), the saved
-- search name (for the digest heading) and query (where a digest too large to carry
-- whole sends its "and N more" tail), the user's account email (the email
-- channel's live recipient), the user's linked Telegram chat (NULL when unlinked
-- → the worker soft-skips telegram delivery rather than failing it), whether
-- the user has at least one registered push device (the push channel's live
-- deliverability check, same soft-skip role as the Telegram link), the user's
-- webhook destination (URL, NULL or disabled → the worker soft-skips webhook
-- delivery the same way; its secret signs the delivery), and the delivery-timing context
-- (live, not snapshotted, same as the channel checks above) — the account's
-- timezone and its saved-search digest frequency settings, read via
-- internal/application/deliverywindow before a digest is sent.
SELECT s.id, s.user_id, s.saved_search_id, s.channel, s.destination, s.last_digest_sent_at,
       ss.name AS saved_search_name,
       ss.query AS saved_search_query,
       mail_recipient(u.email, u.email_verified, u.last_active_at, u.banned_at)::text AS account_email,
       u.timezone AS timezone,
       tl.chat_id AS telegram_chat_id,
       (u.banned_at IS NULL AND EXISTS(SELECT 1 FROM user_push_tokens upt WHERE upt.user_id = s.user_id))::boolean AS has_push_device,
       wc.url AS webhook_url,
       COALESCE(wc.enabled, false) AS webhook_enabled,
       wc.secret AS webhook_secret,
       (CASE WHEN s.channel = 'webhook'
                  OR COALESCE(u.pro_until > now(), false) OR COALESCE(u.ultra_until > now(), false)
             THEN COALESCE(ns.digest_frequency, 'daily') ELSE 'daily' END)::text AS digest_frequency, -- effective, see ListActiveSubscriptions
       COALESCE(ns.digest_time, '09:00'::time) AS digest_time,
       ns.quiet_hours_start AS quiet_hours_start,
       ns.quiet_hours_end AS quiet_hours_end,
       -- When this person was last written to on this channel: the claim's `pl.last_sent`,
       -- the newest stamp among their active subscriptions on it (a webhook keeps its own).
       (SELECT max(o.last_digest_sent_at)
          FROM subscriptions o
         WHERE o.user_id = s.user_id AND o.channel = s.channel AND o.active
           AND (s.channel <> 'webhook' OR o.id = s.id))::timestamptz AS person_last_sent_at
FROM subscriptions s
JOIN saved_searches ss ON ss.id = s.saved_search_id
JOIN users u ON u.id = s.user_id
LEFT JOIN telegram_links tl ON tl.user_id = s.user_id AND u.banned_at IS NULL
LEFT JOIN webhook_configs wc ON wc.user_id = s.user_id AND u.banned_at IS NULL
LEFT JOIN notification_settings ns ON ns.user_id = s.user_id
WHERE s.id = $1;

-- name: ListDeliveryCandidates :many
-- Every active subscription that has a pending match, with the SAME delivery context row as
-- GetSubscriptionForDelivery (column for column: notify converts one into the other, so the
-- build breaks if they drift). The worker decides from it, with deliverywindow and recipient,
-- which of them are deliverable now, and only those go to ClaimSubscriptionMatches: a
-- subscription held by quiet hours, a daily digest time or a missing recipient is never leased.
-- A superset of what the claim can take (no lease or email opt-out test here: the claim keeps
-- those), so a row listed and then not claimed costs a read, never a missed delivery.
SELECT s.id, s.user_id, s.saved_search_id, s.channel, s.destination, s.last_digest_sent_at,
       ss.name AS saved_search_name,
       ss.query AS saved_search_query,
       mail_recipient(u.email, u.email_verified, u.last_active_at, u.banned_at)::text AS account_email,
       u.timezone AS timezone,
       tl.chat_id AS telegram_chat_id,
       (u.banned_at IS NULL AND EXISTS(SELECT 1 FROM user_push_tokens upt WHERE upt.user_id = s.user_id))::boolean AS has_push_device,
       wc.url AS webhook_url,
       COALESCE(wc.enabled, false) AS webhook_enabled,
       wc.secret AS webhook_secret,
       (CASE WHEN s.channel = 'webhook'
                  OR COALESCE(u.pro_until > now(), false) OR COALESCE(u.ultra_until > now(), false)
             THEN COALESCE(ns.digest_frequency, 'daily') ELSE 'daily' END)::text AS digest_frequency, -- effective, see ListActiveSubscriptions
       COALESCE(ns.digest_time, '09:00'::time) AS digest_time,
       ns.quiet_hours_start AS quiet_hours_start,
       ns.quiet_hours_end AS quiet_hours_end,
       -- When this person was last written to on this channel: the claim's `pl.last_sent`,
       -- the newest stamp among their active subscriptions on it (a webhook keeps its own).
       (SELECT max(o.last_digest_sent_at)
          FROM subscriptions o
         WHERE o.user_id = s.user_id AND o.channel = s.channel AND o.active
           AND (s.channel <> 'webhook' OR o.id = s.id))::timestamptz AS person_last_sent_at
FROM subscriptions s
JOIN saved_searches ss ON ss.id = s.saved_search_id
JOIN users u ON u.id = s.user_id
LEFT JOIN telegram_links tl ON tl.user_id = s.user_id AND u.banned_at IS NULL
LEFT JOIN webhook_configs wc ON wc.user_id = s.user_id AND u.banned_at IS NULL
LEFT JOIN notification_settings ns ON ns.user_id = s.user_id
WHERE s.active
  AND EXISTS (
      SELECT 1 FROM subscription_matches mm
      WHERE mm.subscription_id = s.id
        AND mm.notified_at IS NULL
        AND mm.failed_at IS NULL
        AND mm.expired_at IS NULL
        AND mm.query_revision = ss.query_revision
  );

-- name: MarkDigestSent :exec
-- Stamp the subscription's last digest send instant. A `daily` subscription reads it
-- through internal/application/deliverywindow.DigestDue ("already sent today"); an
-- `interval` one through ClaimSubscriptionMatches' elapsed-interval gate. Called after
-- every successful delivery in either mode — `instant`-mode subscriptions never touch
-- this column.
UPDATE subscriptions
SET last_digest_sent_at = now()
WHERE id = $1;

-- name: GetJobsForDigest :many
-- The display fields for the jobs in a digest, freshest first. Salary fields are
-- projected out of the enrichment JSONB (absent keys → NULL) so a card can render
-- a compensation line only when one is known.
SELECT id, title, company, public_slug, url, posted_at,
       COALESCE((enrichment->>'salary_min')::int, 0)::int AS salary_min,
       COALESCE((enrichment->>'salary_max')::int, 0)::int AS salary_max,
       COALESCE(enrichment->>'salary_currency', '')::text AS salary_currency,
       COALESCE(enrichment->>'salary_period', '')::text   AS salary_period
FROM jobs
WHERE id = ANY(sqlc.arg(job_ids)::bigint[])
ORDER BY COALESCE(posted_at, created_at) DESC;

-- name: MarkMatchesNotified :execrows
-- Stamp notified_at on the jobs that were just delivered for a subscription, so
-- they leave the pending queue and are never sent again.
UPDATE subscription_matches
SET notified_at = now()
WHERE subscription_id = sqlc.arg(subscription_id)
  AND job_id = ANY(sqlc.arg(job_ids)::bigint[]);

-- name: RecordMatchDeliveryFailure :exec
-- Count a failed delivery for a subscription's claimed jobs: bump attempts, record
-- the error, and dead-letter (set failed_at) once attempts reach the max. claimed_at
-- is left in place — its expiry gates the retry to a later pass and doubles as the
-- crash reaper, mirroring enrichment_outbox.
UPDATE subscription_matches
SET attempts   = attempts + 1,
    last_error = sqlc.arg(last_error),
    failed_at  = CASE
                     WHEN attempts + 1 >= sqlc.arg(max_attempts)::int THEN now()
                     ELSE NULL
                 END
WHERE subscription_id = sqlc.arg(subscription_id)
  AND job_id = ANY(sqlc.arg(job_ids)::bigint[]);

-- name: ReleaseMatchClaim :exec
-- Release the lease on a subscription's claimed jobs without counting an attempt,
-- so a soft-skipped delivery (e.g. Telegram not yet linked) is retried promptly on
-- a later pass instead of waiting out the lease.
UPDATE subscription_matches
SET claimed_at = NULL
WHERE subscription_id = sqlc.arg(subscription_id)
  AND job_id = ANY(sqlc.arg(job_ids)::bigint[]);
