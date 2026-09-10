## Context

See `proposal.md` — Why. The relevant code today:

- `Service.CheckoutURL` (`internal/identity/billing/service.go`) always calls
  `client.createCheckoutSession`, which always opens a new Stripe Checkout Session
  (`mode=subscription`), reusing an existing Stripe *customer* id but never an existing
  *subscription*.
- `client.subscriberState` already reads a customer's subscriptions, but the parsed
  `subscription` struct (`internal/identity/billing/entitlement.go`) carries no subscription
  id or item id — only status, dates, and price ids — because nothing needed to name a
  subscription to modify it before now.
- `entitlement.go`'s `bestEntitling`/`billedSubscription` already resolve, from a customer's
  raw subscription list, which single subscription "counts" for a given price list. The
  in-code comment on `billedSubscription` already describes the intended shape — "an upgrade
  that adds the new price to the existing subscription rather than opening a second one" —
  which is the behavior this change implements; nothing in the codebase did that yet.
- `internal/identity/billing/AGENTS.md`'s "What is deliberately absent" section rules out "a
  cancellation flow of our own." That constraint is untouched: this change never cancels a
  subscription, it changes what an *existing, still-open* subscription is billed for.

## Goals / Non-Goals

**Goals:**
- A signed-in customer with an active entitling subscription who requests a different price
  (a different tier, or a different interval of the same tier) ends the request with exactly
  one active entitling subscription, changed in place with proration.
- A customer with no active entitling subscription is unaffected: they still go through
  `createCheckoutSession`, exactly as today.
- A customer who is already found to have more than one active entitling subscription (the
  pre-existing-bug state) is told so on their billing overview, instead of the overview
  silently picking one and hiding the other.

**Non-Goals:**
- Cleaning up subscriptions that are *already* duplicated for existing customers. That is an
  operator action (Stripe dashboard or Customer Portal), out of scope for this change.
- Applying a promo discount/coupon to an in-place tier change. `Discount` continues to apply
  only to a brand-new Checkout Session; see Decisions.
- Any change to the cancellation flow. That continues to be Stripe's Customer Portal, linked
  via `ManagementURL`.

## Decisions

**Decide purely, act separately.** The choice of what to do — open a new checkout, update an
existing subscription in place, or do nothing because the customer already has the requested
price — is a pure function of the customer's current subscriptions and the requested price
id, with no I/O. It lives in `entitlement.go` beside `bestEntitling`/`billedSubscription`,
which already make the same kind of decision, and is unit-tested the same way (table-driven,
no database, no network — see `TestBilledSubscription`/`TestProUntilFrom` for the existing
pattern). `CheckoutURL` becomes a thin orchestrator: read the customer's subscriptions, call
the pure function, then either call `createCheckoutSession` or the new
`client.updateSubscriptionPrice`.

Alternative considered: decide inline inside `CheckoutURL`. Rejected because `CheckoutURL`
needs `*db.Queries` for `GetStripeCustomerID`/`UserEmail`, which makes it untestable without a
real database (there is no existing unit test for it beyond the disabled-service case) —
exactly the gap that let this bug ship unnoticed. Pulling the decision out is what makes it
testable at all.

**Modify in place via Stripe's subscription-item replacement, not the Customer Portal's
`subscription_update` flow.** `POST /v1/subscriptions/{id}` with `items[0][id]`,
`items[0][price]` and `proration_behavior=create_prorations` changes the existing
subscription's price server-side, synchronously, and returns to the same `/my/plan` page the
checkout flow already returns to.

Alternative considered: redirect to a Customer Portal session opened with
`flow_data[type]=subscription_update`. Rejected for now — it would return control to Stripe's
hosted UI for a click the user already made on our own pricing page (confirming a choice
they already confirmed), and `createPortalSession` today opens a portal with no `flow_data`
at all, which would need its own plumbing. The direct API call keeps the existing UX (click
"Upgrade to Ultra", land back on `/my/plan`) unchanged for the customer.

**Which subscription counts, when a customer might already have more than one:** the pure
decision function asks across *both* configured price lists (`Config.Prices` +
`Config.UltraPrices`) for the customer's single best-entitling subscription (reusing
`bestEntitling`), the same tier-agnostic selection `billedSubscription` already makes. A
customer already duplicated (the pre-existing bug) has this function pick one — the
furthest-reaching — and leaves the other untouched; see Non-Goals.

**A subscription's item id is the FIRST item's id.** Every subscription this integration
creates carries exactly one item (`createCheckoutSession` sends one `line_items[0]`), and
after this change an upgrade/downgrade replaces that one item's price rather than adding a
second item — so a subscription this integration manages never grows a second item going
forward. Reading only `items.data[0].id` is therefore not a simplification that could regress
silently; it matches the one invariant this integration maintains.

**No discount/coupon on an in-place update.** `CheckoutURL` still accepts a `Discount` and
still applies it — via `createCoupon` + `discounts[0][coupon]` — only on the
new-Checkout-Session branch, exactly as today. A discount is a reason to make a *first*
purchase (the promo/invite-reward system's own framing); extending it to a tier change already
in progress is a product question this bug fix does not need to answer, and skipping it keeps
the change's surface small.

**The billing overview reports a boolean, not a list.** `Overview` gains
`MultipleSubscriptions bool` (`omitempty`), decided by counting *distinct subscription ids*
(not price ids — one subscription can legitimately carry items of more than one tier per the
existing `billedSubscription` comment) that entitle across both price lists. The overview
still resolves and shows the single best-entitling one for its status/amount/date, exactly as
today; the boolean is additive information, not a second code path.

## Risks / Trade-offs

- **[Risk]** A customer already duplicated by the bug, who now triggers another tier change,
  gets only their best-entitling subscription updated — the other keeps billing. →
  **Mitigation:** the overview's new `multiple_subscriptions` flag makes this visible on
  `/my/plan` (frontend surfaces a message pointing at "Manage subscription", which already
  links to the Customer Portal where the extra subscription can be cancelled by hand). Full
  automated remediation of existing duplicates is explicitly out of scope (see Non-Goals).
- **[Risk]** `client.subscriberState` now runs on the checkout path for every returning
  customer (previously it only ran on `/billing/subscription` and the webhook/reconciler
  paths), adding one provider round trip and one more way `CheckoutURL` can fail for an
  existing customer. → **Mitigation:** failing this read fails the whole call (an error, not a
  fallback to creating a new checkout) — deliberately, because falling back would silently
  recreate the exact bug being fixed. A first-time buyer (no stored customer id) is unaffected
  and never takes this path.
- **[Trade-off]** Proration on an in-place upgrade charges/credits the difference
  immediately (Stripe's `create_prorations` default), which is a different billing experience
  from "a whole new subscription starting today, old one still running until it lapses." This
  is the correct behavior for the bug being fixed (only one subscription should ever exist)
  and matches how Stripe's own Customer Portal upgrade flow already behaves.

## Migration Plan

- No schema change and no data migration. `internal/identity/billing/AGENTS.md`'s "no local
  subscription state machine" rule holds: everything this change reads and writes lives at
  Stripe.
- Deploy as an ordinary backend + frontend change; no ordering constraint against other
  in-flight work.
- Rollback is a plain revert: the previous behavior (always open a new Checkout Session) is
  restored, and no stored data needs to be undone.
- Existing customers already duplicated by the bug are not touched by this change and need a
  manual fix (cancel the extra subscription via the Customer Portal or Stripe dashboard) —
  called out to the user in this conversation for their own account already.
