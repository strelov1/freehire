-- Audience reader and ledger writer for one-off campaigns (internal/engage/broadcast).

-- name: ListBroadcastCandidates :many
-- Everyone who can be mailed and has not received this campaign yet. A non-empty half ('a' or
-- 'b') narrows that to one side of a split test, and a non-empty timezones list to the
-- accounts whose stored timezone is one of them (how a campaign reaches one country).
--
-- No signup window, unlike the onboarding queries: a campaign is addressed to the
-- whole audience by definition, and the launch it announces matters as much to
-- someone who joined in June as to someone who joined last week. The cap is
-- therefore the only bound on a run, which is why the caller passes it explicitly
-- rather than relying on a default.
--
-- The two exclusions are the same as everywhere else, for the same reasons: an
-- unverified address was never proven to belong to anyone, and an explicit
-- notification_settings.news_email_enabled = false is an opt-out. A missing settings
-- row means the account never touched the setting and still hears from us.
--
-- The gate used to be `enabled`, which also governs the lifecycle nudges — so
-- declining letters from the founder also stopped somebody's application
-- follow-up reminders, and an account with no settings row could not decline at
-- all, because the only thing that creates the row is a page behind the login.
-- Migration 0153 split the two.
--
-- An account that already holds today's mail_day_claims row is left out — one optional
-- letter per account per UTC day across all senders — and stays a candidate for the next
-- run. CountBroadcastCandidates does not carry that clause: it answers how much of the
-- audience is still to be reached, and a person who got another letter today is.
SELECT u.id, u.email
FROM users u
LEFT JOIN notification_settings ns ON ns.user_id = u.id
WHERE mail_recipient(u.email, u.email_verified, u.last_active_at, u.banned_at) <> ''
  AND COALESCE(ns.news_email_enabled, true)
  AND NOT EXISTS (
      SELECT 1 FROM broadcast_emails be
      WHERE be.user_id = u.id AND be.campaign = sqlc.arg(campaign)
  )
  AND sqlc.arg(half)::text IN ('', broadcast_half(sqlc.arg(campaign)::text, u.id))
  AND (COALESCE(cardinality(sqlc.arg(timezones)::text[]), 0) = 0 OR u.timezone = ANY(sqlc.arg(timezones)::text[]))
  AND NOT EXISTS (
      SELECT 1 FROM mail_day_claims c WHERE c.user_id = u.id AND c.day = mail_today()
  )
ORDER BY u.created_at
LIMIT sqlc.arg(max_rows)::int;

-- name: CountBroadcastCandidates :one
-- How many people a campaign would reach right now. Read before sending: a campaign
-- is irreversible and goes to everyone, so the number is worth seeing first.
SELECT count(*)
FROM users u
LEFT JOIN notification_settings ns ON ns.user_id = u.id
WHERE mail_recipient(u.email, u.email_verified, u.last_active_at, u.banned_at) <> ''
  AND COALESCE(ns.news_email_enabled, true)
  AND NOT EXISTS (
      SELECT 1 FROM broadcast_emails be
      WHERE be.user_id = u.id AND be.campaign = sqlc.arg(campaign)
  )
  AND sqlc.arg(half)::text IN ('', broadcast_half(sqlc.arg(campaign)::text, u.id))
  AND (COALESCE(cardinality(sqlc.arg(timezones)::text[]), 0) = 0 OR u.timezone = ANY(sqlc.arg(timezones)::text[]));

-- name: RecordBroadcastEmail :exec
-- Closes out one (user, campaign) whether or not the send worked.
INSERT INTO broadcast_emails (user_id, campaign, error)
VALUES (sqlc.arg(user_id), sqlc.arg(campaign), sqlc.arg(error))
ON CONFLICT (user_id, campaign) DO NOTHING;
