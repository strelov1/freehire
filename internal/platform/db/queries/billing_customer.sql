-- name: SetStripeCustomerID :exec
-- Bind an account to the payment provider's customer, once, when it first transacts.
--
-- IS DISTINCT FROM rather than a bare assignment: the webhook and the reconciler both take
-- this path and both will usually be writing the value that is already there, and a write
-- that changes nothing should not wake a trigger or bloat a row.
UPDATE users
SET stripe_customer_id = sqlc.arg(stripe_customer_id)::text
WHERE id = sqlc.arg(id)
  AND stripe_customer_id IS DISTINCT FROM sqlc.arg(stripe_customer_id)::text;

-- name: GetStripeCustomerID :one
-- Which customer to ask the provider about for this account. NULL means the account has
-- never transacted, which is the same answer the provider gives for an id it does not know.
SELECT stripe_customer_id
FROM users
WHERE id = $1;

-- name: GetUserIDByStripeCustomer :one
-- Resolve a provider customer back to one of our accounts. This is the direction a webhook
-- needs: a delivery names the customer, never the user.
--
-- No rows means the customer is not ours — a dashboard test object, or a customer created
-- outside this integration. The caller records the event and reports it rather than
-- guessing.
SELECT id
FROM users
WHERE stripe_customer_id = sqlc.arg(stripe_customer_id)::text;

-- name: ListSubscribersNearProExpiryStripe :many
-- The reconciler's second pass: accounts bound to a provider customer whose plan expiry
-- falls inside a window around now, so a renewal whose webhook was never delivered is
-- repaired on the next run.
--
-- It walks users rather than billing_events because the binding column is what makes the
-- question answerable at all — and it is indexed, so the scan is over the accounts that
-- have transacted rather than over all of them.
SELECT id, stripe_customer_id, pro_until
FROM users
WHERE stripe_customer_id IS NOT NULL
  AND pro_until >= sqlc.arg(from_time)
  AND pro_until < sqlc.arg(to_time)
LIMIT sqlc.arg(max_rows);
