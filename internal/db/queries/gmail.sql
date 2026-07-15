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
-- Flat inbox listing, newest first — one row per message (no subject grouping),
-- soft-deleted messages excluded. Optional filters (each empty/false = no filter):
-- source narrows to one account; unread hides already-read mail; status narrows to
-- one classified signal; the search term matches subject, sender, or body. The
-- snippet is the body's leading text with whitespace collapsed, for the list row.
-- The link/classification columns ride alongside so the inbox can render the
-- confirm chip and application link without a second lookup; the LEFT JOINs
-- resolve the linked/suggested application's public slug + company for display.
SELECT emails.id, emails.source, emails.external_id, emails.from_addr, emails.from_name, emails.subject,
    left(regexp_replace(emails.body_text, E'\\s+', ' ', 'g'), 160)::text AS snippet,
    emails.received_at, (emails.read_at IS NOT NULL)::boolean AS read,
    emails.job_id, emails.suggested_job_id, emails.status_signal, emails.link_source,
    lj.public_slug AS linked_slug, lj.company AS linked_company,
    sj.public_slug AS suggested_slug, sj.company AS suggested_company
FROM emails
LEFT JOIN jobs lj ON lj.id = emails.job_id
LEFT JOIN jobs sj ON sj.id = emails.suggested_job_id
WHERE emails.user_id = $1
  AND emails.deleted_at IS NULL
  AND (sqlc.arg(src)::text = '' OR emails.source = sqlc.arg(src))
  AND (sqlc.arg(unread)::bool = false OR emails.read_at IS NULL)
  AND (sqlc.arg(status)::text = '' OR emails.status_signal = sqlc.arg(status))
  AND (
    sqlc.arg(q)::text = ''
    OR emails.subject   ILIKE '%' || sqlc.arg(q) || '%'
    OR emails.from_name ILIKE '%' || sqlc.arg(q) || '%'
    OR emails.from_addr ILIKE '%' || sqlc.arg(q) || '%'
    OR emails.body_text ILIKE '%' || sqlc.arg(q) || '%'
  )
ORDER BY emails.received_at DESC, emails.id DESC
LIMIT sqlc.arg(lim) OFFSET sqlc.arg(off);

-- name: CountEmails :one
-- Total live messages for the caller (same optional filters as ListEmails), for
-- pagination.
SELECT count(*)
FROM emails
WHERE user_id = $1
  AND deleted_at IS NULL
  AND (sqlc.arg(src)::text = '' OR source = sqlc.arg(src))
  AND (sqlc.arg(unread)::bool = false OR read_at IS NULL)
  AND (sqlc.arg(status)::text = '' OR status_signal = sqlc.arg(status))
  AND (
    sqlc.arg(q)::text = ''
    OR subject   ILIKE '%' || sqlc.arg(q) || '%'
    OR from_name ILIKE '%' || sqlc.arg(q) || '%'
    OR from_addr ILIKE '%' || sqlc.arg(q) || '%'
    OR body_text ILIKE '%' || sqlc.arg(q) || '%'
  );

-- name: GetEmail :one
SELECT emails.id, emails.source, emails.external_id, emails.s3_key, emails.from_addr, emails.from_name, emails.subject,
    emails.body_text, emails.body_html, emails.received_at, (emails.read_at IS NOT NULL)::boolean AS read,
    emails.job_id, emails.suggested_job_id, emails.status_signal, emails.link_source,
    lj.public_slug AS linked_slug, lj.company AS linked_company,
    sj.public_slug AS suggested_slug, sj.company AS suggested_company
FROM emails
LEFT JOIN jobs lj ON lj.id = emails.job_id
LEFT JOIN jobs sj ON sj.id = emails.suggested_job_id
WHERE emails.id = $1 AND emails.user_id = $2 AND emails.deleted_at IS NULL;

-- name: MarkEmailRead :exec
-- Stamp read on first open; a no-op once already read.
UPDATE emails SET read_at = now()
WHERE id = $1 AND user_id = $2 AND read_at IS NULL;

-- name: MarkAllEmailsRead :execrows
-- Bulk mark-as-read for the caller, honoring the same optional filters as the
-- listing, so "mark all read" means "everything currently shown". Only unread,
-- live rows are touched; returns how many it marked.
UPDATE emails SET read_at = now()
WHERE user_id = $1
  AND read_at IS NULL
  AND deleted_at IS NULL
  AND (sqlc.arg(src)::text = '' OR source = sqlc.arg(src))
  AND (sqlc.arg(status)::text = '' OR status_signal = sqlc.arg(status))
  AND (
    sqlc.arg(q)::text = ''
    OR subject   ILIKE '%' || sqlc.arg(q) || '%'
    OR from_name ILIKE '%' || sqlc.arg(q) || '%'
    OR from_addr ILIKE '%' || sqlc.arg(q) || '%'
    OR body_text ILIKE '%' || sqlc.arg(q) || '%'
  );

-- name: SoftDeleteEmail :execrows
-- Soft-delete one message (hidden from the listing, retained for restore),
-- scoped to the caller and idempotent. Returns 0 rows only when it is not the
-- caller's message (→ 404).
UPDATE emails SET deleted_at = now()
WHERE id = $1 AND user_id = $2;

-- name: RestoreEmail :execrows
-- Undo a soft-delete, scoped to the caller and idempotent. Returns 0 rows only
-- when it is not the caller's message (→ 404).
UPDATE emails SET deleted_at = NULL
WHERE id = $1 AND user_id = $2;
