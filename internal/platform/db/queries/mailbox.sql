-- name: GetMailboxByUser :one
-- Existence check: does this account have a hosted mailbox at all. address is not
-- selected — it is retired in favor of the derived <users.username>@<domain> address
-- (see the add-username-claim change) and no longer read by any caller.
SELECT id, user_id, created_at FROM mailboxes WHERE user_id = $1;

-- name: ListMailboxesWithoutBackfilledUsername :many
-- Candidates for cmd/backfill-username-from-mailbox: every hosted mailbox whose
-- owner has not yet been backfilled onto users.username. Small by construction —
-- mailboxes is an opt-in feature, nowhere near the row counts the repo's chunked
-- cmd/backfill-* workers exist for — so one unpaged query is enough.
SELECT m.user_id, m.address
FROM mailboxes m
JOIN users u ON u.id = m.user_id
WHERE u.username IS NULL
ORDER BY m.user_id;

-- name: EnsureMailbox :exec
-- Enroll userID in the hosted mailbox, idempotently — a second call for the same
-- user is a no-op. The address itself is never stored here; it is always derived
-- from the account's username (see the add-username-claim change).
INSERT INTO mailboxes (user_id) VALUES ($1)
ON CONFLICT (user_id) DO NOTHING;

-- name: DeleteMailbox :exec
DELETE FROM mailboxes WHERE user_id = $1;

-- name: InsertHostedMessage :exec
-- Store a message received at a hosted mailbox, idempotent by
-- (user_id, source, external_id) with source fixed to 'hosted'.
INSERT INTO emails (
    user_id, source, external_id, s3_key, from_addr, from_name,
    subject, body_text, body_html, received_at, ical_uid
) VALUES ($1, 'hosted', $2, $3, $4, $5, $6, $7, $8, $9, sqlc.arg(ical_uid))
ON CONFLICT (user_id, source, external_id) DO NOTHING;
