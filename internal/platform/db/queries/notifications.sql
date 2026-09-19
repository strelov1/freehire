-- name: RecordNotification :one
-- Record one delivered notify/reminder/nudge event for the in-app notification
-- center, independent of which channel(s) carried it. Called right alongside
-- each engine's own "marked delivered" write; a failure here must never fail
-- the delivery it accompanies (see the add-notification-center design). jobs
-- is only ever set by a multi-job subscription digest (see 0091); every other
-- kind, and a single-job digest, passes NULL and relies on public_slug instead.
--
-- Returns the new row's id because a subscription digest is recorded BEFORE it
-- is sent, so the message can link to this row's matched-jobs page. Reminders
-- and nudges record after delivery as before and discard the id.
--
-- dedup_key names the EVENT this row records, for the callers whose delivery
-- runs once per CHANNEL rather than once per event — today only the
-- subscription digest, whose `subscriptions` row is keyed (saved_search_id,
-- channel). Passing one makes the write claim-or-yield: the first channel to
-- arrive inserts, and every later channel carrying the same event conflicts and
-- inserts nothing, returning NO ROW (sqlc.ErrNoRows) so the caller can tell
-- "I recorded this" from "somebody already had". Every engine that already
-- records once per event passes NULL and behaves exactly as before, since the
-- unique index behind the conflict target is partial on dedup_key IS NOT NULL.
INSERT INTO user_notifications (user_id, kind, title, body, public_slug, jobs, dedup_key)
VALUES ($1, $2, $3, $4, sqlc.narg(public_slug), sqlc.narg(jobs), sqlc.narg(dedup_key))
ON CONFLICT (user_id, dedup_key) WHERE dedup_key IS NOT NULL DO NOTHING
RETURNING id;

-- name: GetNotificationIDByDedupKey :one
-- The id of the row already recorded for an event, for the channel that lost
-- the race to record it and still needs the id to link its "and N more" tail at.
--
-- Deliberately a second round trip rather than folding it into
-- RecordNotification as an ON CONFLICT DO UPDATE ... RETURNING id: the upsert
-- form always returns an id and so cannot say WHICH caller created the row,
-- and that distinction is what stops a later channel's failed send from
-- withdrawing the history row an earlier channel's successful send earned.
SELECT id FROM user_notifications
WHERE user_id = sqlc.arg(user_id) AND dedup_key = sqlc.arg(dedup_key);

-- name: DeleteNotification :exec
-- Remove a notification recorded for a delivery that then failed, so the
-- history holds a row for a digest if and only if the digest went out. Only the
-- record-before-send path (subscription digests) can need this.
DELETE FROM user_notifications WHERE id = $1;

-- name: ListUserNotifications :many
-- The caller's own notifications, newest first, standard offset/limit paging
-- (matching every other /me/* list endpoint in this codebase).
SELECT id, kind, title, body, public_slug, jobs, created_at, read_at
FROM user_notifications
WHERE user_id = $1
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: GetNotification :one
-- One notification, owner-scoped (no row for another user's id, mapped to 404
-- by the handler) — the direct-link/detail read a bookmarked or freshly
-- visited /my/notifications/[id]/jobs page needs, since ListUserNotifications
-- alone only serves the caller's own current page of the list.
SELECT id, kind, title, body, public_slug, jobs, created_at, read_at
FROM user_notifications
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);

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
