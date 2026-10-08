-- name: GetWebhookConfig :one
-- The user's webhook destination, if any. No row means the user has never
-- configured one — the caller (both the settings API and delivery's recipient
-- resolution) treats absence as "not configured" rather than an error.
SELECT user_id, url, enabled, created_at, updated_at, last_success_at, disabled_at, consecutive_failures
FROM webhook_configs
WHERE user_id = $1;

-- name: UpsertWebhookConfig :one
-- Creates the account's webhook destination, or updates its URL if one
-- already exists — there is exactly one row per user (see migration 0135).
-- Saving re-enables a previously disabled destination and clears
-- disabled_at, since submitting the form is an explicit re-commitment to
-- the endpoint. consecutive_failures resets too: a new URL (or the same one
-- re-saved) gets a fresh run at the auto-disable threshold, not whatever
-- count the old destination left behind.
INSERT INTO webhook_configs (user_id, url, enabled, updated_at, disabled_at)
VALUES ($1, $2, true, now(), NULL)
ON CONFLICT (user_id) DO UPDATE
SET url = EXCLUDED.url,
    enabled = true,
    disabled_at = NULL,
    consecutive_failures = 0,
    updated_at = now()
RETURNING user_id, url, enabled, created_at, updated_at, last_success_at, disabled_at, consecutive_failures;

-- name: EnableWebhookConfig :one
-- Re-enables a user-disabled (or auto-disabled) webhook destination without
-- changing its URL. Resets consecutive_failures for the same reason
-- UpsertWebhookConfig does: re-enabling is a fresh start, not a standing
-- invitation to immediately re-trip the threshold that just disabled it.
UPDATE webhook_configs
SET enabled = true,
    disabled_at = NULL,
    consecutive_failures = 0,
    updated_at = now()
WHERE user_id = $1
RETURNING user_id, url, enabled, created_at, updated_at, last_success_at, disabled_at, consecutive_failures;

-- name: DisableWebhookConfig :execrows
-- Disables the destination, stamping disabled_at. Used both by the settings
-- API (user-initiated) and by the notify delivery engine when a send gets a
-- definitive 410 Gone from the destination (see internal/engage/webhooknotify).
-- Returns the affected row count: 0 means there was no destination to disable
-- (or it was already disabled by an earlier subscription in the same pass).
UPDATE webhook_configs
SET enabled = false,
    disabled_at = now(),
    updated_at = now()
WHERE user_id = $1 AND enabled;

-- name: RecordWebhookDeliveryFailure :one
-- Counts a non-410 send failure (404, 500, timeout, ...) toward the
-- destination's auto-disable threshold and disables it once the new count
-- reaches max_failures — the counterpart to the 410 path in
-- DisableWebhookConfig, for a destination that fails without ever saying so
-- outright. Unlike subscription_matches.attempts (per match, reset by every
-- new job), this counts consecutively across matches, so a destination that
-- is simply dead gets disabled once instead of failing forever.
UPDATE webhook_configs
SET consecutive_failures = consecutive_failures + 1,
    enabled = CASE WHEN consecutive_failures + 1 >= sqlc.arg(max_failures)::bigint THEN false ELSE enabled END,
    disabled_at = CASE WHEN consecutive_failures + 1 >= sqlc.arg(max_failures)::bigint THEN now() ELSE disabled_at END,
    updated_at = now()
WHERE user_id = sqlc.arg(user_id)
RETURNING consecutive_failures, enabled;

-- name: RecordWebhookDeliverySuccess :exec
-- Stamps last_success_at after a delivery succeeds and resets
-- consecutive_failures — a success means the destination is not, in fact,
-- dead, so a failure before it must not count toward disabling it after.
-- Not gated on `enabled` — a disabled destination is never delivered to
-- (soft-skipped upstream), so this only ever runs for an enabled one.
UPDATE webhook_configs
SET last_success_at = now(),
    consecutive_failures = 0
WHERE user_id = $1;

-- name: DeleteWebhookConfig :execrows
DELETE FROM webhook_configs
WHERE user_id = $1;
