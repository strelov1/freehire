-- name: GetMailboxByUser :one
SELECT id, user_id, address, created_at FROM mailboxes WHERE user_id = $1;

-- name: GetMailboxByAddress :one
-- Recipient resolution for the inbound ingest worker.
SELECT id, user_id, address, created_at FROM mailboxes WHERE address = $1;

-- name: InsertMailbox :one
-- Claim an address for a user. May raise a unique violation on user_id (already
-- has a mailbox) or address (taken) — the allocation service handles both: it
-- reads-back on a user conflict and retries the next suffix on an address conflict.
INSERT INTO mailboxes (user_id, address) VALUES ($1, $2)
RETURNING id, user_id, address, created_at;

-- name: DeleteMailbox :exec
DELETE FROM mailboxes WHERE user_id = $1;

-- name: InsertHostedMessage :exec
-- Store a message received at a hosted mailbox, idempotent by
-- (user_id, source, external_id) with source fixed to 'hosted'.
INSERT INTO emails (
    user_id, source, external_id, s3_key, from_addr, from_name,
    subject, subject_norm, body_text, body_html, received_at
) VALUES ($1, 'hosted', $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (user_id, source, external_id) DO NOTHING;
