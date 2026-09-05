-- name: GetWebhookConfig :one
-- The user's webhook destination, if any. No row means the user has never
-- configured one — the caller (both the settings API and delivery's recipient
-- resolution) treats absence as "not configured" rather than an error.
SELECT user_id, url, enabled, created_at, updated_at, last_success_at, disabled_at
FROM webhook_configs
WHERE user_id = $1;

-- name: UpsertWebhookConfig :one
-- Creates the account's webhook destination, or updates its URL if one
-- already exists — there is exactly one row per user (see migration 0135).
-- Saving re-enables a previously disabled destination and clears
-- disabled_at, since submitting the form is an explicit re-commitment to
-- the endpoint.
INSERT INTO webhook_configs (user_id, url, enabled, updated_at, disabled_at)
VALUES ($1, $2, true, now(), NULL)
ON CONFLICT (user_id) DO UPDATE
SET url = EXCLUDED.url,
    enabled = true,
    disabled_at = NULL,
    updated_at = now()
RETURNING user_id, url, enabled, created_at, updated_at, last_success_at, disabled_at;

-- name: EnableWebhookConfig :one
-- Re-enables a user-disabled (or auto-disabled) webhook destination without
-- changing its URL.
UPDATE webhook_configs
SET enabled = true,
    disabled_at = NULL,
    updated_at = now()
WHERE user_id = $1
RETURNING user_id, url, enabled, created_at, updated_at, last_success_at, disabled_at;

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

-- name: RecordWebhookDeliverySuccess :exec
-- Stamps last_success_at after a delivery succeeds. Not gated on `enabled` —
-- a disabled destination is never delivered to (soft-skipped upstream), so
-- this only ever runs for an enabled one.
UPDATE webhook_configs
SET last_success_at = now()
WHERE user_id = $1;

-- name: DeleteWebhookConfig :execrows
DELETE FROM webhook_configs
WHERE user_id = $1;
