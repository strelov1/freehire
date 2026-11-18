-- name: UpsertPushToken :one
-- Register (or refresh) a device's push token. A token identifies one device
-- install, not one user, so the conflict target is the token itself: if it
-- already belongs to a different account, this reassigns it to the caller
-- rather than duplicating or leaving it with the previous owner.
INSERT INTO user_push_tokens (user_id, token, platform, last_seen_at)
VALUES (sqlc.arg(user_id), sqlc.arg(token), sqlc.arg(platform), now())
ON CONFLICT (token) DO UPDATE
  SET user_id      = EXCLUDED.user_id,
      platform     = EXCLUDED.platform,
      last_seen_at = now()
RETURNING *;

-- name: ListPushTokensForUser :many
-- The caller's own registered devices, for a test send or a future delivery. A banned
-- account has none: every push sender reads its devices here, so this is the one gate.
SELECT t.* FROM user_push_tokens t
JOIN users u ON u.id = t.user_id
WHERE t.user_id = $1 AND u.banned_at IS NULL;

-- name: UserHasPushToken :one
-- Whether the account has at least one registered device — the precondition for
-- choosing the push channel at all, since push delivers to nothing else.
SELECT EXISTS (SELECT 1 FROM user_push_tokens WHERE user_id = $1);

-- name: DeletePushToken :execrows
-- Explicit unregistration (sign-out), scoped to the caller so one account
-- cannot unregister another's device.
DELETE FROM user_push_tokens WHERE user_id = sqlc.arg(user_id) AND token = sqlc.arg(token);

-- name: PruneDeadPushToken :exec
-- Removes a token the Expo Push API reported as permanently undeliverable
-- (DeviceNotRegistered). No owner check: the token is dead regardless of
-- who currently holds it. This is the notifier's internal cleanup path, not
-- a caller-facing "delete a token" request — that goes through
-- DeletePushToken's owner check instead.
DELETE FROM user_push_tokens WHERE token = $1;
