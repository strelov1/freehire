-- Candidate readers for the founder signup sequence (internal/onboarding).
--
-- Three rules hold across all of them, and each is load-bearing:
--
--   * `email_verified` — an unverified address is one nobody proved they own.
--     Mailing it is how a sending domain collects bounces and spam complaints for
--     addresses that were typos to begin with.
--   * The `window_days` bound — without it the first deploy mails the entire
--     historical user table at once. It also caps the blast radius of any future
--     mistake to two weeks of signups.
--   * The LEFT JOIN on notification_settings — a missing row means the account
--     never touched the setting, which is not the same as opting out, so it still
--     gets the sequence. An explicit `enabled = false` stops it.

-- name: ListWelcomeCandidates :many
-- Verified accounts inside the window that have not been greeted yet. This is the
-- only step with no waiting period: it goes out on the next pass after signup.
SELECT u.id, u.email
FROM users u
LEFT JOIN notification_settings ns ON ns.user_id = u.id
WHERE u.email_verified
  AND u.created_at > now() - make_interval(days => sqlc.arg(window_days)::int)
  AND COALESCE(ns.enabled, true)
  AND NOT EXISTS (
      SELECT 1 FROM onboarding_emails oe
      WHERE oe.user_id = u.id AND oe.step = 'welcome'
  )
ORDER BY u.created_at
LIMIT sqlc.arg(max_rows)::int;

-- name: ListAdvancedSearchCandidates :many
-- Greeted a while ago: an introduction to the filter panel (role, region, skills,
-- and how to exclude a value). Sent to everyone regardless of alert status, unlike
-- no_alert below — this is background on a feature, not a nudge toward one missing
-- action, so it asks nothing of the reader that would make it conditional.
SELECT u.id, u.email
FROM users u
JOIN onboarding_emails w ON w.user_id = u.id AND w.step = 'welcome'
LEFT JOIN notification_settings ns ON ns.user_id = u.id
WHERE u.email_verified
  AND u.created_at > now() - make_interval(days => sqlc.arg(window_days)::int)
  AND w.sent_at < now() - make_interval(days => sqlc.arg(after_days)::int)
  AND COALESCE(ns.enabled, true)
  AND NOT EXISTS (
      SELECT 1 FROM onboarding_emails oe
      WHERE oe.user_id = u.id AND oe.step = 'advanced_search'
  )
ORDER BY u.created_at
LIMIT sqlc.arg(max_rows)::int;

-- name: ListNoAlertCandidates :many
-- Greeted a while ago, and still without an active alert — the one action the
-- product is built around.
--
-- The wait is measured from the greeting (w.sent_at), not from signup. Measuring it
-- from signup breaks on the very first run: every account older than the delay is
-- instantly eligible for both steps, so the same person gets "hi, I'm Ilya" and then
-- "you signed up a few days ago and still have no alert" an hour apart. From two
-- mails in an hour, a stranger is indistinguishable from a spammer.
SELECT u.id, u.email
FROM users u
JOIN onboarding_emails w ON w.user_id = u.id AND w.step = 'welcome'
LEFT JOIN notification_settings ns ON ns.user_id = u.id
WHERE u.email_verified
  AND u.created_at > now() - make_interval(days => sqlc.arg(window_days)::int)
  AND w.sent_at < now() - make_interval(days => sqlc.arg(after_days)::int)
  AND COALESCE(ns.enabled, true)
  AND NOT EXISTS (
      SELECT 1 FROM subscriptions s WHERE s.user_id = u.id AND s.active
  )
  AND NOT EXISTS (
      SELECT 1 FROM onboarding_emails oe
      WHERE oe.user_id = u.id AND oe.step = 'no_alert'
  )
ORDER BY u.created_at
LIMIT sqlc.arg(max_rows)::int;

-- name: ListOpenSourceCandidates :many
-- Everyone greeted and past the wait, whether or not they set up an alert: this
-- step asks for a star and a Discord visit, which is worth asking of a browser as
-- much as of a regular.
--
-- The wait runs from the greeting, for the same reason it does above: paced from
-- signup, a two-week-old account would receive the whole sequence in one afternoon.
SELECT u.id, u.email
FROM users u
JOIN onboarding_emails w ON w.user_id = u.id AND w.step = 'welcome'
LEFT JOIN notification_settings ns ON ns.user_id = u.id
WHERE u.email_verified
  AND u.created_at > now() - make_interval(days => sqlc.arg(window_days)::int)
  AND w.sent_at < now() - make_interval(days => sqlc.arg(after_days)::int)
  AND COALESCE(ns.enabled, true)
  AND NOT EXISTS (
      SELECT 1 FROM onboarding_emails oe
      WHERE oe.user_id = u.id AND oe.step = 'open_source'
  )
ORDER BY u.created_at
LIMIT sqlc.arg(max_rows)::int;

-- name: RecordOnboardingEmail :exec
-- Closes out one (user, step) whether or not the send worked; `error` carries the
-- transport failure when it did not. ON CONFLICT DO NOTHING makes a double run
-- harmless: the first row stands and the send is never repeated.
INSERT INTO onboarding_emails (user_id, step, error)
VALUES (sqlc.arg(user_id), sqlc.arg(step), sqlc.arg(error))
ON CONFLICT (user_id, step) DO NOTHING;
