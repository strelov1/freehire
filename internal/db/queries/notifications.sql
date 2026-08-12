-- name: RecordNotification :exec
-- Record one delivered notify/reminder/nudge event for the in-app notification
-- center, independent of which channel(s) carried it. Called right alongside
-- each engine's own "marked delivered" write; a failure here must never fail
-- the delivery it accompanies (see the add-notification-center design).
INSERT INTO user_notifications (user_id, kind, title, body, public_slug)
VALUES ($1, $2, $3, $4, sqlc.narg(public_slug));

-- name: ListUserNotifications :many
-- The caller's own notifications, newest first, standard offset/limit paging
-- (matching every other /me/* list endpoint in this codebase).
SELECT id, kind, title, body, public_slug, created_at, read_at
FROM user_notifications
WHERE user_id = $1
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountUserNotifications :one
-- Total and unread counts in one statement and one set of predicates, so the
-- two numbers in a list response's meta always describe the same mailbox.
SELECT
    count(*)::bigint AS total,
    count(*) FILTER (WHERE read_at IS NULL)::bigint AS unread
FROM user_notifications
WHERE user_id = $1;

-- name: MarkNotificationRead :execrows
-- Owner-scoped and idempotent in one statement: COALESCE leaves an
-- already-set read_at untouched (so repeating the call doesn't bump the
-- timestamp), while the WHERE clause alone still matches the row — a zero
-- affected-row count therefore unambiguously means the id doesn't exist or
-- isn't the caller's (the handler maps that to 404), never "already read".
UPDATE user_notifications
SET read_at = COALESCE(read_at, now())
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);

-- name: MarkAllNotificationsRead :execrows
-- Bulk mark-as-read for the caller; only unread rows are touched, and the
-- affected count is returned to the client as confirmation.
UPDATE user_notifications
SET read_at = now()
WHERE user_id = $1 AND read_at IS NULL;
