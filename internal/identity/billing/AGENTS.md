# Billing conventions

## Scope

The Pro subscription: where a candidate buys it, and how the provider's record of it
reaches `users.pro_until` — the one column that decides a plan. Nothing else. What the plan
then ALLOWS is `internal/ai/plan`'s business, and the two packages never meet: `plan` reads
the column through `platform/db` and does not import this package.

**This is freehire.me's hosted billing and is not supported for self-hosting.** It is in the
open repository under the same licence as the rest — closing it would protect nothing, since
secrets live in the environment either way — but nothing here is built to be run by anyone
else. Without its environment variables the package reports itself disabled, its routes
answer 404, and its worker exits without opening a connection.

## Always true

- **A webhook is a SIGNAL, never a fact.** The handler does not branch on the event type,
  and there are hundreds of them. It records the delivery and then re-reads the customer's
  current subscriptions, deriving `users.pro_until` from that. Two reasons, both stated in
  the provider's own documentation: delivery is **not ordered**, so a renewal can arrive
  after the cancellation it superseded; and branching on event types builds a copy of the
  provider's state machine here, which is a second source of truth about money.

- **The column is derived whole, never adjusted.** That is what makes a repeat free, an
  out-of-order delivery harmless, and refunds, cancellations and failed cards need no code
  of their own — they are already reflected in what we re-read.

- **Record, answer 200, then apply.** The acknowledgement is a claim that the event is
  durably stored, so it is made once that is true and not before. A failure to APPLY does
  not change the response — the row stays unprocessed and `cmd/billing-sync` picks it up. A
  failure to RECORD does: the delivery goes unacknowledged so the provider retries.

- **Three statuses entitle, and `past_due` is one of them.** The provider retries a failed
  card for days before giving up. Cutting access on the first failed attempt turns a card
  that needs updating into a cancelled customer. The subscription's own period end still
  bounds it, so this can never grant time nobody bought.

- **A cancelled subscription still entitles until its period ends.** Cancelling says "do not
  renew", not "refund me". They bought that time.

- **`current_period_end` lives on the subscription ITEM, not on the subscription**, and
  reading the wrong one is not a cosmetic miss. The provider moved it — a subscription can
  hold items on different cycles, so the field stopped having one answer at the top — and a
  client that reads only the old place gets zero for every subscriber. The first draft turned
  a zero into a never-expires sentinel, which would have made every subscriber permanent and
  stopped cancellation taking effect, silently and in the direction of giving the product
  away. Both places are read now, the furthest wins, and **a period end that cannot be read
  entitles nobody**: a Stripe subscription always has one, so an unreadable end means the
  wrong field, not "forever".

- **An empty price list confers Pro on NOBODY.** A deployment that forgot to name its price
  must refuse to make anyone Pro rather than make everyone Pro — which is why `Enabled()`
  requires the list and there is no default.

- **The signature covers the RAW body — `c.Request().Body()`, NOT Fiber's `Ctx.Body()`.**
  Never a re-marshalled struct either: the same event serialised differently is different
  bytes, and a valid delivery would be rejected. Fiber's `Ctx.Body()` looks like the right
  call and is not — it honours `Content-Encoding` and decompresses, chaining up to three
  layers. On the one unauthenticated POST in the app that is a hole with two sides: what the
  HMAC covers becomes a header the sender writes, and the decoding happens BEFORE the
  signature is checked while the server's 8MB `BodyLimit` bounds only the compressed body, so
  a few megabytes of brotli from anyone at all are an unbounded allocation. The scheme is
  HMAC-SHA256 over `"<t>.<raw body>"` with the endpoint secret, and **every scheme but `v1`
  is ignored** — the provider's own instruction, and a downgrade defence: they send a
  deliberately fake `v0` on test deliveries, so a verifier that accepted any scheme would
  accept a signature it never checked.

- **EVERY `v1` element is tried, not just one.** While two endpoint secrets are active the
  provider signs each delivery with both and sends two `v1`s in one header, in no promised
  order — so a verifier keeping a single one rejects roughly half of all deliveries for the
  length of the rollover. Which is exactly when a secret is being rotated because it leaked.

- **A refusal to VERIFY is 401; a refusal to PARSE is 400.** Both are permanent, and the
  status is the only way to say so. Answering a body that can never parse with something the
  provider treats as retryable is how an endpoint gets retried for three days and then
  disabled, taking the renewals with it. `ErrBadSignature` is what the handler branches on.

- **The customer binding is WRITE-ONCE** (`SetStripeCustomerID` is `AND stripe_customer_id
  IS NULL`), and that is a security property. `resolveUser` falls back to the account
  reference the provider echoes back whenever no binding exists yet, and on the Payment Link
  path that reference is attacker-supplied — `?client_reference_id=` is a query parameter
  anyone opening the link may set. A binding that could be REPLACED would let somebody paying
  for their own subscription repoint another person's account at their customer: the victim's
  own subscription is orphaned (nothing reads a customer no user points at, so the reconciler
  never touches it again) and their plan then follows the attacker's card. Refusing costs
  nothing the system needs — the self-healing rebind in `Apply` is precisely the NULL case,
  and an account that genuinely has to move is a support ticket and one `UPDATE`.

- **The public price cache never holds its lock across a provider call, and a FAILURE expires
  it too.** `/api/v1/plans` is public, unauthenticated and unrate-limited, and most of the
  traffic reaching it is crawlers. A lock spanning the network serialises every visitor
  behind one third-party request; a failure that does not set a backoff turns each of those
  page views into another call to a provider that is already struggling, spending the API
  rate limit the webhook's own reads need. One refresh is in flight at a time and everyone
  else is served the held answer.

- **The signed timestamp is checked against a five-minute window** — the provider's own
  default tolerance. Without it a captured delivery is a bearer credential that replays
  forever. The window bounds the age of the SIGNATURE, not of the event: a retry is
  re-signed when it is sent, so widening it to admit a late retry would only lengthen a
  captured delivery's life.

- **The account id travels in TWO places, and both earn their keep.**
  `client_reference_id` comes back on the completed checkout — the ONE event that arrives
  before any customer binding exists. The customer's metadata survives that event, which is
  how every later renewal is attributed. Neither alone covers the whole life of a
  subscription.

- **`users.stripe_customer_id` exists because the reconciler runs the OTHER way round.** A
  webhook names a customer and we look up the user; a scheduled re-check starts from a user
  and has to name the customer. Without the stored binding that direction has no answer.

- **Checkout reuses a known customer.** A second purchase that created a second customer for
  one person would leave two subscriptions nobody sums.

- **Absent credentials mean disabled, never an error.** `ConfigFromEnv` cannot fail and `New`
  cannot fail. `Enabled()` gates the subsystem; `CanCheckout()` additionally needs a site URL
  to return a buyer to, and is separate on purpose — subscriptions already sold keep
  renewing, and refusing to record their renewals because nobody can start a NEW purchase
  would lose money we have been paid.

## The provider contract, as read

From Stripe's documentation on **2026-09-03**. Re-read it if the client starts failing in a
way the tests do not reproduce.

- Envelope: `{"id": "evt_…", "type": "…", "data": {"object": {…}}}`. Redeliveries reuse the
  id, which is what makes it the idempotency key.
- Signature: `Stripe-Signature: t=<unix>,v1=<hex>[,v0=…]`, HMAC-SHA256 over
  `"<t>.<raw body>"` with the `whsec_` endpoint secret.
- Subscriptions: `GET /v1/subscriptions?customer=…&status=all&expand[]=data.items.data.price`
  with `Authorization: Bearer sk_…`. **The price must be expanded** or the items carry a bare
  id string with nothing to match on, and **`current_period_end` is on the item** — verified
  against a real subscription rather than a stub, which is the only way that one shows up.
- Checkout: `POST /v1/checkout/sessions`, form-encoded. Management: `POST
  /v1/billing_portal/sessions`, which returns a short-lived URL.
- Delivery: unordered, duplicates possible, retried for up to three days; an endpoint the
  provider decides is broken can be disabled sooner than that.

## Placement

The `identity` block, layer 3: a subscription is an attribute of the account. The constraint
that pushed `internal/ai/plan` out of this block — `ai` and `identity` share a layer, so
`ai/assistant` could not import it — does not reach here. `plan` never imports this package,
and billing's callers are the webhook handler in `api` and a binary in `cmd/`, both of which
may import anything.

## What is deliberately absent

**A provider interface.** There is one provider on the web. The `billing_events.provider`
column already carries the seam, because a second one is a real prospect rather than a
hypothetical: mobile purchases cannot go through Stripe at all, so the day an app ships,
events from somewhere else land in the same table beside these. Writing the interface before
that provider is chosen would be a guess at its shape.

**A local subscription state machine.** No status, no period, no price stored. The provider
owns those and they are one API call away; a copy here could only disagree.

**A cancellation flow of our own.** The provider's portal owns it, and we link to it. A flow
we wrote would be one more thing that can disagree with what actually happened to the money.
