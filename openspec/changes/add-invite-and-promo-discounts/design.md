## Context

Pro is sold through one Stripe checkout session created by `billing.Service.CheckoutURL`,
and entitlement is derived by re-reading the provider (`internal/identity/billing/AGENTS.md`).
Nothing in that path can move a price, and nothing anywhere tracks who brought whom.

Three constraints shape everything below.

**The repository is public.** A discount whose codes, seat limits or reward rules can be read
out of the source is drained the day someone greps it. Every figure that decides money lives
in the database or the host environment.

**`billing`'s scope is narrow on purpose**, and its documentation says so: the Pro
subscription and how a provider's record of it reaches that provider's source column,
nothing else. Discounts are a reason to buy, not a fact about a subscription. Putting the
promo rules in `billing` would make that document false.

**The webhook path must not grow work that can fail.** `engine.Apply` records, re-reads, and
stamps; a failure there costs a reconciler pass and nothing else. It is also
provider-generic, and a referral reward is Stripe-only — a store subscription produces no
invoice we can read.

## Goals / Non-Goals

**Goals:**

- One mechanism — "a discount on this account's next invoice" — serving promo codes, the
  invitee's first-month discount, and the referrer's reward.
- Every code, limit and amount readable only from the database or the environment.
- Abuse bounded by constraints and by the requirement that real money moved, not by
  heuristics about emails or addresses.
- `billing` and the new package independent of each other in both directions.

**Non-Goals:**

- Discounts on store subscriptions (App Store, Google Play). RevenueCat's path is untouched.
- An admin UI for promo codes. An `INSERT` is the interface.
- Recurring or multi-month discounts. Every discount here is `duration: once`.
- Cash payouts, affiliate commissions, or a balance a user can withdraw. The reward is
  credit against our own invoices and nothing else.

## Decisions

### The package is `internal/identity/promo`, not an extension of `engage/referral`

`internal/engage/referral` is the employee-referral marketplace — offers to refer someone
into a company, moderation, anonymity. It shares a word with this feature and nothing else.
Reusing it would put two unrelated domains behind one name in a repository where package
placement is enforced by a layering table.

`promo` sits in the `identity` block at layer 3: a discount is an attribute of an account.
It imports `platform/db` and nothing else in the module. **It must be added to the table in
`internal/platform/arch/layering/blocks.go`** — a package in neither table fails depguard.

*Alternative considered:* put it in `billing`. Rejected: it would make billing's own scope
document false, and would tie the promo rules to a package whose tests stand a Stripe stub in
front of every case.

### `promo` decides, `billing` executes, and neither imports the other

`promo` answers one question — what discount, if any, does this account get on this
purchase — and returns a plain value. `billing` gains a `Discount` value type of its own and
an extra parameter on `CheckoutURL`; it never learns why the discount exists.

```go
// billing
type Discount struct {
    PercentOff int32  // 1..100; zero means no discount
    Label      string // what the buyer sees on the coupon
    Key        string // idempotency key for minting the coupon
}
```

`Key` is not decoration: a coupon is minted per checkout, so a retried or double-clicked
purchase without one leaves a second coupon behind for a single sale. It is derived from the
account, the source and the percentage — the same buyer asking for the same offer twice gets
the same coupon back from the provider.

A percentage and nothing else — see the delivery decision below for why a referral reward
never arrives as a coupon. The HTTP handler asks `promo`, hands the answer to
`billing`, and that is the only place the two meet. Both are in the `identity` block, so a
direct import would pass depguard — the separation is a scope decision, not a layering one,
and the seam is the plain value type.

*Alternative considered:* `billing` calls into `promo` through an interface it defines.
Rejected as one indirection more than the problem has: there is exactly one caller, and it
is a handler that already holds both.

### The referrer's reward is granted by a worker pass, not by the webhook

`cmd/billing-sync` already runs hourly, already holds `DATABASE_URL` and the Stripe
credentials, and already exists to apply what the webhook could not. It gains a third pass:
take `invite_rewards` still `pending` whose invitee could plausibly have paid, ask the
provider whether any of that invitee's invoices collected a non-zero amount, and grant.

Three reasons over hooking `engine.Apply`:

- `Apply` is provider-generic and this is Stripe-only. A hook there would have to ask which
  provider it is running under, which is the branch the whole design avoids.
- The grant is idempotent by its status guard, so a pass that crashes half-way costs a
  repeat that changes nothing — the same property that makes `Apply` safe.
- The webhook is answered inside a window the provider enforces. Adding a second provider
  round-trip to it trades a delivery for an hour of latency on a referral reward.

**And a fourth, operational one: a new binary would need a new systemd unit, and units live
only on the production host** — `release.sh` flips the app and never touches one
(`deploy/AGENTS.md`). Extending a worker that already has a timer is a deploy; adding one is
a deploy plus a manual host step that is easy to forget and silent when forgotten.

The candidate set is small and bounded: pending rewards whose invitee holds a
`stripe_customer_id`. `INVITE_REWARD_MAX_PER_RUN` bounds one pass.

*Alternative considered:* a callback on `Service` invoked after a successful apply. Rejected
for the reasons above, and because it would make the webhook's success depend on a table
that has nothing to do with the subscription it is recording.

### "Has paid" means an invoice that collected, not a subscription that is active

`amount_paid > 0` on at least one of the invitee's invoices. A subscription can be active
having collected nothing — a trial, or the 90% code stacked with something — and paying a
reward for that turns the discount into a way to mint credit.

The read is `GET /v1/invoices?customer=…&limit=…`, a new client method beside
`subscriberState`. It follows the same rule as everything else in that file: the provider is
the source of truth and we re-read it rather than keeping a copy.

### The reward amount is fixed when it is granted

50% of the list price, resolved from the price cache at grant time and stored in
`invite_rewards.amount_cents`. A later price change must not silently revalue credit somebody
has already earned, in either direction.

### Delivery is always a balance credit, and the customer is created if there is not one

A negative customer balance transaction. The provider consumes it on the next invoice, which
is the only mechanism that does not require knowing when the next invoice is due — and it is
also, word for word, what the reward promises: half off next month.

A referrer who has never bought has no customer to credit, so **one is created for them**
(`POST /v1/customers`) and bound through the existing write-once `SetStripeCustomerID`. The
credit then sits on it until they subscribe, and their first invoice consumes it.

This replaces an earlier draft in which an undelivered reward became an `amount_off` coupon
at the referrer's own checkout. That draft had a hole with no cheap fix: the credit had to be
marked consumed when the session was created, but a session is abandoned far more often than
it is completed, so an abandoned checkout burned the reward. Deferring the consumption until
the invoice was paid would mean tracking which session carried which reward — a second
ledger, disagreeing with the provider's. Creating the customer early costs one API call for
somebody who has already earned a reward, which means somebody else has already paid us.

Because of this, **there is exactly one kind of discount attached to a checkout session: a
percentage.** Credit is never a coupon, so the question of which of two discounts wins does
not arise, and `Discount` carries no amount.

`POST /v1/customers/{id}/balance_transactions` needs an `Idempotency-Key` header, which the
current `client.do` cannot set. It gains an optional header argument — the narrowest change
that serves both this and coupon creation.

### Seats are claimed by the statement that tests them

```sql
UPDATE promo_codes
   SET uses = uses + 1
 WHERE code = $1
   AND active
   AND (expires_at IS NULL OR expires_at > now())
   AND (max_uses IS NULL OR uses < max_uses)
RETURNING percent_off;
```

No row back means refused, for whatever reason. A read-then-write would let two accounts take
the last seat of a 200-seat launch offer, and the moment that matters is exactly the moment
the offer is popular.

### One redemption per account, one reward per invitee — as constraints

`promo_redemptions` is unique on `user_id` (not on `(code, user_id)`): an account redeems one
code in its lifetime, so a second offer cannot be stacked onto the first.
`invite_rewards.referee_id` is unique, so an account can never be the subject of a second
reward however many times it is attributed. Both are constraints rather than checks, because
a check that runs before a write is a race and a constraint is not.

### The code is carried by a server-set cookie, and nothing else

This is the part that decides whether the feature works at all, because the gap between
clicking an invite link and creating an account is where attribution is normally lost.

**A cookie, not `localStorage`.** The reference implementation this feature was modelled on
keeps the code in `localStorage` and sends it in the body of the checkout request. That works
there because it attributes at CHECKOUT — an ordinary POST the client composes. It cannot
work here, because we attribute at REGISTRATION, and the majority registration path is
OAuth: the visitor leaves for the provider and comes back on a **GET redirect, which has no
body to carry anything**. A value only the browser's JavaScript can see is a value the OAuth
callback cannot read. The cookie travels on that redirect by itself.

**Set by the server, never by `document.cookie`.** Safari's tracking prevention caps
JavaScript-written cookies at seven days. A 30-day window only survives if the cookie arrives
as a `Set-Cookie` header, which is why this lives in `hooks.server.ts` and not in a component.
httpOnly follows for free, and means an XSS cannot read or forge an attribution.

**Captured from any URL shape.** `hooks.server.ts` reads `?ref=` on *every* request, so
`freehire.me/?ref=x`, `freehire.me/jobs/some-job?ref=x` and a bare `/r/x` all work. The
dedicated `/r/[code]` route exists only to make the link short and to redirect to the site
root. Handling `?ref=` in one hook rather than on each page is what stops a shared deep link
from silently attributing nothing.

**First toucher wins.** The cookie is written only when absent. Otherwise a second invite
link — or a link a bad actor gets a pending invitee to open — would overwrite an attribution
already earned.

**Cleared only once it has done its job.** Both registration paths (the password handler and
the OAuth callback) read the cookie, attribute, and expire it in the same response. If
attribution fails for any reason, the cookie stays and the next request tries again. Nothing
clears it speculatively.

`?promo=` gets the same treatment in a separate cookie, read by the pricing page's server
`load` to prefill the field. It is not httpOnly, because prefilling is its whole purpose and
the value is a code the visitor already has.

*Deliberately not added: a `localStorage` mirror.* It could only help when a first-party
httpOnly cookie is dropped — and in that browser the session cookie is dropped too, so the
visitor cannot be signed in at all. It would be a second source of truth covering nothing.

*Alternative considered:* a Go route at `/r/`. Rejected — it is a top-level path outside
`/api/v1`, so it needs an nginx change on the production host, the same "lives only on the
host" hazard as a new systemd unit, for no gain.

*Consent:* the cookie is first-party, functional, and set because the visitor deliberately
followed a link offering them a discount. It carries no identifier of the visitor — only the
referrer's public code — and reaches no third party. It is therefore not gated behind the
analytics consent banner (`web/src/lib/consent.ts`), which governs trackers.

### The ceiling is an environment variable

`INVITE_REWARD_MAX_PER_USER`, default 12 (≈ half a year free). An unparseable or non-positive
value falls back to the default and logs, rather than failing the worker: a typo here must
not stop the pass that also reconciles subscriptions.

This is the one number the user asked to keep in the database. It is in the environment
instead because every other operational bound in this repository is (`BILLING_SYNC_MAX_PER_RUN`,
`HYDRATION_RETRY_DAYS`, `APPLY_FORM_CONCURRENCY`), the host `.env` changes without a deploy,
and a table holding a single integer would be a second place to look for a number.

### No redeemable code ships in the repository

A test walks the module for string literals shaped like a promo code and fails naming the
file. Fixtures use codes the schema accepts but a prefix the test allowlists, so the guard
cannot be satisfied by weakening it.

## Risks / Trade-offs

- **Attribution is a cookie the visitor controls, so anyone can claim any referrer.** →
  Accepted, because the cookie only decides *who is credited*, never *whether money moved*.
  The self-referral case — a second account, a real card — costs the abuser a real payment to
  earn 50% of one month, which is not an arbitrage. The per-referrer ceiling bounds the
  patient version.

- **A reward granted, then the invitee refunds.** → Accepted loss, bounded by the reward
  size. Clawing back a delivered balance credit would mean writing a second money state
  machine to disagree with the provider's, which `billing/AGENTS.md` rejects for good reason.
  Recorded as an open question rather than silently ignored.

- **A referrer who never subscribes accrues credit that is never delivered.** → By design;
  the invite page reports it as pending, which is also the incentive to subscribe. It costs
  nothing until they do.

- **A coupon object per checkout accumulates in the provider's account.** → Bounded by an
  idempotency key derived from the account, the code and the price, so retries and
  double-clicks reuse one. Coupons are free and unlimited; the cost is clutter, not money.

- **The reward lands up to an hour after the invitee pays.** → Accepted for a referral
  reward. The invitee's own discount is immediate, which is the half that affects a purchase
  decision.

- **`client.do` grows a header parameter, touching every existing call site.** → A small,
  mechanical change, covered by the package's existing tests. The alternative — a parallel
  `doIdempotent` — is a second copy of the error handling that would drift.

## Migration Plan

1. Migration `0140_promo_and_invites.sql` creates the four tables. Additive only; no applied
   file is edited. `pnpm check:sql` lints it; `make sqlc` regenerates `internal/platform/db`.
2. Deploy. With no rows in `promo_codes` and no minted invite codes, every new surface is
   inert: checkout behaves exactly as it does today.
3. Set `INVITE_REWARD_MAX_PER_USER` in the host `.env` if the default is not wanted.
4. `INSERT` the launch code (90%, capped seats, an expiry) when the offer starts. Its
   spelling is chosen then and lives only in the database — naming it here would put a
   redeemable code in a public repository, which is the one thing this design forbids.
5. **Rollback** is `UPDATE promo_codes SET active = false` — it stops new redemptions
   immediately without a deploy. Rewards already granted stay granted; that is the point of
   granting on money.

## Open Questions

- **Should a refund inside the first billing period void an undelivered reward?** It is one
  extra condition on the worker pass for the undelivered case, and impossible for the
  delivered one. Deferred until there is a first refund to look at.
- **Does the invite page need a per-invitee list?** The spec deliberately reports counts
  only, since naming who accepted an invite discloses that someone signed up for a job board.
  Revisit only if the counts prove too thin to be motivating.
