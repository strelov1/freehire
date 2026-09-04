-- name: SetStripeCustomerID :exec
-- Bind an account to the payment provider's customer, ONCE, when it first transacts.
--
-- IS NULL, so this writes a binding and never REPLACES one. That is a security property,
-- not a tidiness one. An account's customer is resolved two ways (see Service.resolveUser):
-- from the stored binding, and — only when there is no binding yet — from the account
-- reference the provider echoes back. That reference is attacker-supplied on one path: a
-- Stripe Payment Link takes `?client_reference_id=` from whoever opens it. With a bare
-- assignment, somebody who paid for their own subscription while naming SOMEBODY ELSE'S
-- account id would overwrite that account's binding, orphan the subscription it was
-- actually paying for — nothing reads a customer no user points at, so the reconciler
-- never touches it again — and then hold the victim's plan hostage to their own card.
--
-- Refusing the write costs nothing the system needs. The self-healing rebind in
-- Service.Apply is precisely the NULL case, and an account that genuinely has to move to a
-- new customer is a support ticket and one UPDATE, not a path a webhook should offer.
--
-- Also subsumes the cheaper reason the predicate was here before: the webhook and the
-- reconciler both take this path and both usually write the value already present, and a
-- write that changes nothing should not wake a trigger or bloat a row.
UPDATE users
SET stripe_customer_id = sqlc.arg(stripe_customer_id)::text
WHERE id = sqlc.arg(id)
  AND stripe_customer_id IS NULL;

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
