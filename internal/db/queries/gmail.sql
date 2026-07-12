-- name: GetGmailConnection :one
SELECT user_id, email, status, sync_cursor, connected_at, last_synced_at
FROM gmail_connections
WHERE user_id = $1;

-- name: GetGmailRefreshToken :one
SELECT refresh_token_enc, status, sync_cursor
FROM gmail_connections
WHERE user_id = $1;

-- name: UpsertGmailConnection :exec
-- Connect (or reconnect) a user's Gmail: store the encrypted refresh token and
-- mark connected, preserving the sync cursor on reconnect.
INSERT INTO gmail_connections (user_id, email, refresh_token_enc, status)
VALUES ($1, $2, $3, 'connected')
ON CONFLICT (user_id) DO UPDATE
SET email = EXCLUDED.email,
    refresh_token_enc = EXCLUDED.refresh_token_enc,
    status = 'connected';

-- name: ListConnectedGmailUsers :many
-- Drives the sync worker: every connection still authorized.
SELECT user_id, email, sync_cursor
FROM gmail_connections
WHERE status = 'connected';

-- name: SetGmailSynced :exec
UPDATE gmail_connections
SET sync_cursor = $2, last_synced_at = now()
WHERE user_id = $1;

-- name: SetGmailStatus :exec
UPDATE gmail_connections SET status = $2 WHERE user_id = $1;

-- name: DeleteGmailConnection :exec
DELETE FROM gmail_connections WHERE user_id = $1;

-- name: DeleteUserEmails :exec
DELETE FROM emails WHERE user_id = $1;

-- name: UpsertEmail :exec
-- Idempotent by (user_id, gmail_msg_id): a re-sync of the same message is a no-op.
INSERT INTO emails (
    user_id, gmail_msg_id, thread_id, from_addr, from_name,
    subject, subject_norm, body_text, body_html, received_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (user_id, gmail_msg_id) DO NOTHING;

-- name: ListInboxGroups :many
-- One row per normalized subject: count, newest receipt, distinct sender names,
-- and the newest message's original subject for display. An optional search term
-- (empty = no filter) matches a message's subject, sender, or body — a group
-- surfaces when any of its messages matches.
SELECT
    subject_norm,
    count(*)                                                   AS message_count,
    max(received_at)::timestamptz                              AS latest_received,
    ((array_agg(subject ORDER BY received_at DESC))[1])::text  AS latest_subject,
    array_remove(array_agg(DISTINCT from_name), '')::text[]    AS senders
FROM emails
WHERE user_id = $1
  AND (
    sqlc.arg(q)::text = ''
    OR subject   ILIKE '%' || sqlc.arg(q) || '%'
    OR from_name ILIKE '%' || sqlc.arg(q) || '%'
    OR from_addr ILIKE '%' || sqlc.arg(q) || '%'
    OR body_text ILIKE '%' || sqlc.arg(q) || '%'
  )
GROUP BY subject_norm
ORDER BY max(received_at) DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountInboxGroups :one
-- Total distinct subject groups for the caller (with the same optional search),
-- so the inbox knows whether more pages remain.
SELECT count(DISTINCT subject_norm)
FROM emails
WHERE user_id = $1
  AND (
    sqlc.arg(q)::text = ''
    OR subject   ILIKE '%' || sqlc.arg(q) || '%'
    OR from_name ILIKE '%' || sqlc.arg(q) || '%'
    OR from_addr ILIKE '%' || sqlc.arg(q) || '%'
    OR body_text ILIKE '%' || sqlc.arg(q) || '%'
  );

-- name: ListEmailsByGroup :many
SELECT id, gmail_msg_id, from_addr, from_name, subject, received_at
FROM emails
WHERE user_id = $1 AND subject_norm = $2
ORDER BY received_at DESC;

-- name: GetEmail :one
SELECT id, gmail_msg_id, from_addr, from_name, subject, body_text, body_html, received_at
FROM emails
WHERE id = $1 AND user_id = $2;
