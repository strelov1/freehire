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
--
-- SCOPED BY PROVIDER, and that is not tidiness. An event can only be applied by the provider
-- that sent it: applying is re-reading the subscriber and writing that provider's source
-- column, and the two address accounts differently. Handed another provider's row, a pass
-- would resolve the account through the wrong route, write the wrong column, and then STAMP
-- the row processed — so a store purchase would be marked done having never conferred
-- anything, and never be retried.
SELECT id, provider, event_id, app_user_id, user_id, event_type, received_at
FROM billing_events
WHERE processed_at IS NULL
  AND provider = sqlc.arg(provider)::text
ORDER BY received_at
LIMIT sqlc.arg(max_rows);

-- name: MarkBillingEventProcessed :exec
-- Stamp an event as applied. Idempotent by shape: re-stamping a processed row writes the
-- same fact, and the reconciler's own query no longer returns it.
UPDATE billing_events
SET processed_at = now()
WHERE id = sqlc.arg(id);

-- ListSubscribersNearProExpiry was removed by the add-store-purchases change, and its absence
-- is worth a line because deleting a query is unusual here.
--
-- It predicated on users.pro_until, which stopped being a provider's own answer the moment
-- that column became GREATEST of three sources: a subscriber whose OTHER source reaches
-- further would sit outside the window and never be re-checked, and the lost renewal the pass
-- exists to repair would stay lost. Each provider now has its own near-expiry read against its
-- own column — ListSubscribersNearProExpiryStripe in billing_customer.sql, and
-- ListSubscribersNearProExpiryRevenueCat below.
--
-- Left in place it would have been a generated, callable query that is wrong by construction,
-- sitting where the next provider would reach for it.


-- name: DeleteBillingEventsForUser :exec
-- Erase one user's billing events. Account deletion calls this; the foreign key cascades,
-- but deletion states what it erases explicitly rather than relying on a constraint to
-- mean it.
DELETE FROM billing_events WHERE user_id = $1;

-- name: HasRevenueCatFootprint :one
-- Whether this account has ever been seen by RevenueCat: a recorded delivery of theirs, or a
-- store entitlement we already hold.
--
-- IT GUARDS A READ THAT WRITES. RevenueCat's v1 subscribers endpoint CREATES the subscriber
-- when the identifier is unknown, so asking about an account that never bought anything
-- registers it with the provider. Without this predicate a reconciler pass over the user
-- table would enrol every account we have, silently and permanently.
--
-- Two sources rather than one because they cover different moments: the event row exists from
-- the first delivery onward, and the column survives even if events are ever pruned.
SELECT EXISTS (
    SELECT 1 FROM billing_events
    WHERE user_id = sqlc.arg(user_id)::bigint AND provider = 'revenuecat'
) OR EXISTS (
    SELECT 1 FROM users
    WHERE id = sqlc.arg(user_id)::bigint AND pro_until_revenuecat IS NOT NULL
);

-- name: ListSubscribersNearProExpiryRevenueCat :many
-- The reconciler's second pass for the store provider: accounts whose store entitlement
-- expires inside a window around now, so a renewal whose webhook was never delivered is
-- repaired on the next run.
--
-- The predicate is on pro_until_revenuecat and never on the derived pro_until, for the reason
-- the Stripe query states: the derived column is the FURTHEST of three sources, so a
-- subscriber whose web subscription or manual grant reaches beyond this renewal would sit
-- outside the window and never be re-checked.
--
-- A non-NULL column is also exactly the footprint HasRevenueCatFootprint looks for, so this
-- pass can never ask the provider about an account it would thereby create.
SELECT id
FROM users
WHERE pro_until_revenuecat >= sqlc.arg(from_time)
  AND pro_until_revenuecat < sqlc.arg(to_time)
LIMIT sqlc.arg(max_rows);
