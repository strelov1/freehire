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

-- name: DeleteEmailsBySource :exec
-- Purge one source's mail for a user (Gmail disconnect passes 'gmail', mailbox
-- release passes 'hosted') — the other source's mail is left untouched.
DELETE FROM emails WHERE user_id = $1 AND source = $2;

-- name: UpsertEmail :exec
-- Store a Gmail message, idempotent by (user_id, source, external_id) with
-- source fixed to 'gmail'; the hosted path has its own insert (InsertHostedMessage).
INSERT INTO emails (
    user_id, source, external_id, thread_id, from_addr, from_name,
    subject, body_text, body_html, received_at
) VALUES ($1, 'gmail', $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (user_id, source, external_id) DO NOTHING;

-- name: ListEmails :many
-- Flat inbox listing, newest first — one row per message (no subject grouping).
-- An optional source filter (empty = all accounts) narrows to one source; an
-- optional search term (empty = no filter) matches subject, sender, or body. The
-- snippet is the body's leading text with whitespace collapsed, for the list row.
SELECT id, source, external_id, from_addr, from_name, subject,
    left(regexp_replace(body_text, E'\\s+', ' ', 'g'), 160)::text AS snippet,
    received_at, (read_at IS NOT NULL)::boolean AS read
FROM emails
WHERE user_id = $1
  AND (sqlc.arg(src)::text = '' OR source = sqlc.arg(src))
  AND (
    sqlc.arg(q)::text = ''
    OR subject   ILIKE '%' || sqlc.arg(q) || '%'
    OR from_name ILIKE '%' || sqlc.arg(q) || '%'
    OR from_addr ILIKE '%' || sqlc.arg(q) || '%'
    OR body_text ILIKE '%' || sqlc.arg(q) || '%'
  )
ORDER BY received_at DESC, id DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountEmails :one
-- Total messages for the caller (same optional source + search), for pagination.
SELECT count(*)
FROM emails
WHERE user_id = $1
  AND (sqlc.arg(src)::text = '' OR source = sqlc.arg(src))
  AND (
    sqlc.arg(q)::text = ''
    OR subject   ILIKE '%' || sqlc.arg(q) || '%'
    OR from_name ILIKE '%' || sqlc.arg(q) || '%'
    OR from_addr ILIKE '%' || sqlc.arg(q) || '%'
    OR body_text ILIKE '%' || sqlc.arg(q) || '%'
  );

-- name: GetEmail :one
SELECT id, source, external_id, s3_key, from_addr, from_name, subject,
    body_text, body_html, received_at, (read_at IS NOT NULL)::boolean AS read
FROM emails
WHERE id = $1 AND user_id = $2;

-- name: MarkEmailRead :exec
-- Stamp read on first open; a no-op once already read.
UPDATE emails SET read_at = now()
WHERE id = $1 AND user_id = $2 AND read_at IS NULL;
