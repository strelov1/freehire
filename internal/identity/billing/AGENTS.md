# Billing conventions

## Scope

The Pro subscription: where a candidate buys it, and how the provider's record of it
reaches `users.pro_until` — the one column that decides a plan. Nothing else. What the
plan then ALLOWS is `internal/ai/plan`'s business, and the two packages never meet: `plan`
reads the column through `platform/db` and does not import this package.

**This is freehire.me's hosted billing and is not supported for self-hosting.** It is in
the open repository under the same licence as the rest — closing it would protect nothing,
since secrets live in the environment either way — but nothing here is built to be run by
anyone else. Without its environment variables the package reports itself disabled, its
routes answer 404, and its worker exits without opening a connection.

## Always true

- **A webhook is a SIGNAL, never a fact.** The handler does not branch on the event type.
  It records the delivery and then re-reads the subscriber's current state from the
  provider, deriving `users.pro_until` from that. Two reasons, both confirmed in the
  provider's own documentation: delivery is **not ordered**, so a `RENEWAL` can arrive
  after the `EXPIRATION` it superseded; and branching on event types builds a copy of the
  provider's state machine here, which is a second source of truth about money that drifts
  every time they add or reinterpret a type.

- **The column is derived whole, never adjusted.** That is what makes a repeat free, an
  out-of-order delivery harmless, and refunds, transfers and grace periods need no code of
  their own — they are already reflected in what we re-read.

- **Record, answer 200, then apply.** The acknowledgement is a claim that the event is
  durably stored, so it is made once that is true and not before. A failure to APPLY does
  not change the response — the row stays unprocessed and `cmd/billing-sync` picks it up.
  A failure to RECORD does: the delivery goes unacknowledged so the provider retries.

- **The provider gives up after five attempts over about two and a half hours.** Delivery
  is not a guarantee with a long tail; it is a guarantee that expires. After that the
  reconciler is the only path by which a paid subscription becomes Pro, which is why it is
  scheduled rather than optional.

- **This package speaks API v2, and that was not a style choice.** The project's secret
  key is a v2 `sk_` key and v1 refuses it outright — *"You're trying to use a secret API
  key incompatible with RevenueCat API V1"*, HTTP 403. The first version of this package
  spoke v1 and would have answered 403 to every sync, on every purchase, while every test
  stayed green. Every v2 call is scoped to a project, hence `REVENUECAT_PROJECT_ID`.

- **A customer the provider has never seen answers 404, and that is an ANSWER.** It means
  no purchase, which derives to no entitlement and the free plan. Treating it as a failure
  would leave events unprocessed forever for every identifier that was never ours. Note
  that v2's read does NOT create the customer, unlike v1's — the reconciler still reaches
  candidates through `billing_events` because that is also the cheaper query, but it is no
  longer load-bearing for correctness.

- **A null `expires_date` means never-expiring, not expired.** Reading it as the zero time
  would silently downgrade a lifetime purchaser. `neverExpires` is the sentinel, and it is
  safe only because the column is derived: a refund removes the entitlement and the next
  sync clears it. **The catalogue already carries a `lifetime` non_consumable** beside the
  monthly and yearly subscriptions, so this is a live case rather than a defensive one —
  and a subscriber it got wrong would simply read as free, with nobody looking.

- **The v2 list carries only ACTIVE entitlements**, which removes work rather than adding
  it: lapsed, refunded and transferred all look the same — the entitlement is simply not
  there — and the provider has already applied its own grace-period rule to what remains.
  The v1 shape needed both an expiry and a grace field plus a rule to combine them; there
  is nothing left here to combine.

- **`entitlement_id` is matched against BOTH names the provider has for one entitlement.**
  It carries a human lookup key (`freehire Pro`) and an internal id (`entl…`), the payload
  names it with one of them, and which one is not something to discover from a production
  incident. `REVENUECAT_ENTITLEMENT` therefore holds both, comma-separated — the matching
  logic already took a list.

- **The HMAC covers the RAW body.** Verify `c.Body()`, never a re-marshalled struct — the
  same event serialised differently is different bytes, and a valid delivery would be
  rejected. The signed timestamp is checked against a five-minute window, because a
  signature with no time bound is a bearer credential, which is the property that made the
  shared `Authorization` header worth replacing in the first place. The window bounds the
  age of the SIGNATURE, not of the event: a retry is re-signed when it is sent, so a
  window wide enough for an 80-minute retry would only lengthen a captured delivery's life.

- **The checkout identifier is a PATH SEGMENT.** `https://pay.rev.cat/<token>/<user id>`,
  not a query parameter. `CheckoutURLFor` takes an `int64` rather than a string so the
  app-user identifier cannot be anything but the account's own: the provider assigns an
  anonymous identifier to a client it has not been told about, and a purchase made under
  one lands on a subscriber nobody can resolve to a person.

- **Absent credentials mean disabled, never an error.** `ConfigFromEnv` cannot fail and
  `New` cannot fail. `Enabled()` gates the subsystem; `CanCheckout()` additionally needs a
  paywall URL, and is separate on purpose — a missing web paywall must not stop the webhook
  recording App Store purchases we have already been paid for.

## The provider contract, as read

From RevenueCat's documentation on **2026-09-03**. Re-read it if the client starts failing
in a way the tests do not reproduce.

- Envelope: `{"api_version": "1.0", "event": {…}}`; the event carries `id` (**retries reuse
  it**), `app_user_id`, `type`, `expiration_at_ms`, `entitlement_ids`.
- Signature: `X-RevenueCat-Webhook-Signature: t=<unix>,v1=<hex>`, HMAC-SHA256 over
  `"<t>.<raw body>"`.
- Customer: `GET https://api.revenuecat.com/v2/projects/{project_id}/customers/{id}`,
  `Authorization: Bearer sk_…` (a **secret** key; a public one is refused, and so is a v2
  key sent at v1). It returns `active_entitlements.items[]`, each with `entitlement_id` and
  `expires_at` — **milliseconds since the epoch, nullable**. 404 means the customer has
  never purchased. The v2 customer carries NO `management_url`; v1's did.
- Delivery: unordered, duplicates possible, 60-second response budget, five retries at 5,
  10, 20, 40 and 80 minutes.

## Placement

The `identity` block, layer 3: a subscription is an attribute of the account. The
constraint that pushed `internal/ai/plan` out of this block — `ai` and `identity` share a
layer, so `ai/assistant` could not import it — does not reach here. `plan` never imports
this package, and billing's callers are the webhook handler in `api` and a binary in
`cmd/`, both of which may import anything.

## What is deliberately absent

**A provider interface.** There is one provider. A second would introduce the port; writing
it now would be a guess at the shape of a provider whose documentation nobody has read.

**A local subscription state machine.** No status, no period, no product. The provider owns
those and they are one API call away; a copy here could only disagree.

**Cancellation and refunds, and a link to where they happen.** The provider owns them, and
v2's customer object gives us no URL to point at — v1's `management_url` did, and the
delete-account dialog linked to it precisely so the destination came from them. Composing
one ourselves would reintroduce the staleness that avoided, so the dialog now states that
deleting an account does not cancel a subscription and stops there.

**Gifted Pro days.** `add-invites` grants days of Pro, and if it writes `pro_until` this
package's reconciler will overwrite them with the provider's truth. The resolution is a
separate column for granted days with `pro_until` derived as the later of the two. There
are no gifts yet, so building the split now would be infrastructure ahead of need — but a
change that adds gifts and not the split is a change that loses them.
