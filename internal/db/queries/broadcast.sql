-- Audience reader and ledger writer for one-off campaigns (internal/broadcast).

-- name: ListBroadcastCandidates :many
-- Everyone who can be mailed and has not received this campaign yet.
--
-- No signup window, unlike the onboarding queries: a campaign is addressed to the
-- whole audience by definition, and the launch it announces matters as much to
-- someone who joined in June as to someone who joined last week. The cap is
-- therefore the only bound on a run, which is why the caller passes it explicitly
-- rather than relying on a default.
--
-- The two exclusions are the same as everywhere else, for the same reasons:
-- an unverified address was never proven to belong to anyone, and an explicit
-- notification_settings.enabled = false is an opt-out. A missing settings row means
-- the account never touched the setting and still hears from us.
SELECT u.id, u.email
FROM users u
LEFT JOIN notification_settings ns ON ns.user_id = u.id
WHERE u.email_verified
  AND COALESCE(ns.enabled, true)
  AND NOT EXISTS (
      SELECT 1 FROM broadcast_emails be
      WHERE be.user_id = u.id AND be.campaign = sqlc.arg(campaign)
  )
ORDER BY u.created_at
LIMIT sqlc.arg(max_rows)::int;

-- name: CountBroadcastCandidates :one
-- How many people a campaign would reach right now. Read before sending: a campaign
-- is irreversible and goes to everyone, so the number is worth seeing first.
SELECT count(*)
FROM users u
LEFT JOIN notification_settings ns ON ns.user_id = u.id
WHERE u.email_verified
  AND COALESCE(ns.enabled, true)
  AND NOT EXISTS (
      SELECT 1 FROM broadcast_emails be
      WHERE be.user_id = u.id AND be.campaign = sqlc.arg(campaign)
  );

-- name: RecordBroadcastEmail :exec
-- Closes out one (user, campaign) whether or not the send worked.
INSERT INTO broadcast_emails (user_id, campaign, error)
VALUES (sqlc.arg(user_id), sqlc.arg(campaign), sqlc.arg(error))
ON CONFLICT (user_id, campaign) DO NOTHING;
