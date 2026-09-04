-- name: InsertBillingEvent :one
-- Record a received webhook event, once. See the add-pro-subscription change.
--
-- ON CONFLICT DO NOTHING against the (provider, event_id) unique index is the whole of the
-- idempotency: the provider retries a delivery it did not get a 200 for and reuses the
-- same event id, and says outright that duplicates are possible and delivery is unordered.
-- A redelivery therefore returns NO ROWS, which the caller reads as "already recorded" and
-- answers 200 to — not as an error.
--
-- user_id is passed already resolved and may be NULL: a dashboard TEST event, an event for
-- an account since deleted, and an identifier that was never one of ours all still get
-- recorded. A row we cannot attribute is evidence; a row we refused to write is nothing.
INSERT INTO billing_events (provider, event_id, app_user_id, user_id, event_type, payload)
VALUES (
    sqlc.arg(provider)::text,
    sqlc.arg(event_id)::text,
    sqlc.arg(app_user_id)::text,
    sqlc.narg(user_id),
    sqlc.arg(event_type)::text,
    sqlc.arg(payload)
)
ON CONFLICT (provider, event_id) DO NOTHING
RETURNING id;

-- name: ListUnprocessedBillingEvents :many
-- The reconciler's first pass: events recorded but never applied, oldest first, served by
-- the partial index on (received_at) WHERE processed_at IS NULL.
--
-- Rows with a NULL user_id come back too. They are not skipped in SQL because "an event we
-- could not attribute" is a decision the worker should make visibly and log, not something
-- a WHERE clause silently disposes of.
SELECT id, provider, event_id, app_user_id, user_id, event_type, received_at
FROM billing_events
WHERE processed_at IS NULL
ORDER BY received_at
LIMIT sqlc.arg(max_rows);

-- name: MarkBillingEventProcessed :exec
-- Stamp an event as applied. Idempotent by shape: re-stamping a processed row writes the
-- same fact, and the reconciler's own query no longer returns it.
UPDATE billing_events
SET processed_at = now()
WHERE id = sqlc.arg(id);

-- name: ListSubscribersNearProExpiry :many
-- The reconciler's second pass: subscribers whose plan expiry falls inside a window around
-- now, so a renewal whose webhook was never delivered is repaired within an hour.
--
-- IT WALKS billing_events, NOT users. Two reasons, and the second is the one that matters.
--
-- Cheapness: only an account that has actually transacted appears here, so the candidate
-- set is the subscriber base rather than the 8M-row users table, and it is reached through
-- an index that already exists. A predicate on users.pro_until would want an index on
-- users, and building one on a table that size means either blocking writes to the account
-- table or a CONCURRENTLY build with its own failure mode.
--
-- Correctness: reading a subscriber's state from the provider CREATES that subscriber if
-- the identifier is unknown to them — a GET with a write's consequences. Starting from
-- events makes "we only ever ask about someone who has transacted" a property of the
-- query rather than a rule the worker has to remember.
SELECT DISTINCT b.user_id, u.pro_until
FROM billing_events b
JOIN users u ON u.id = b.user_id
WHERE b.user_id IS NOT NULL
  AND u.pro_until >= sqlc.arg(from_time)
  AND u.pro_until < sqlc.arg(to_time)
LIMIT sqlc.arg(max_rows);

-- name: DeleteBillingEventsForUser :exec
-- Erase one user's billing events. Account deletion calls this; the foreign key cascades,
-- but deletion states what it erases explicitly rather than relying on a constraint to
-- mean it.
DELETE FROM billing_events WHERE user_id = $1;
