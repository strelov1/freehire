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

- **`GET /v1/subscribers/{id}` CREATES the subscriber if the id is unknown** — a read with
  a write's consequences. `Sync` refuses an identifier that does not parse as one of our
  `users.id`, and the reconciler's candidate query starts from `billing_events` rather than
  from `users`, so "we only ask about someone who has transacted" is structural rather than
  remembered.

- **A null `expires_date` means never-expiring, not expired.** Reading it as the zero time
  would silently downgrade a lifetime purchaser. `neverExpires` is the sentinel, and it is
  safe only because the column is derived: a refund removes the entitlement and the next
  sync clears it. We sell no lifetime product, which is exactly why this must be right —
  the case will first occur by accident and nobody will be looking for it.

- **An entitlement reaches the LATER of its expiry and its grace period.** A grace period
  is the provider saying the payment failed but the subscriber is still entitled; reading
  the expiry alone takes access from someone who has it, over a card that needs renewing.

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
- Subscriber: `GET https://api.revenuecat.com/v1/subscribers/{id}`, `Authorization: Bearer
  sk_…` (a **secret** key; a public one is refused). Entitlements are a map keyed by
  entitlement id, each with `expires_date`, `grace_period_expires_date`,
  `product_identifier`, `purchase_date`, in ISO 8601. The response also carries
  `management_url` — the destination the delete-account surface links to, rather than one
  we composed.
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

**Cancellation and refunds.** The provider's `management_url` owns them. We link to it.

**Gifted Pro days.** `add-invites` grants days of Pro, and if it writes `pro_until` this
package's reconciler will overwrite them with the provider's truth. The resolution is a
separate column for granted days with `pro_until` derived as the later of the two. There
are no gifts yet, so building the split now would be infrastructure ahead of need — but a
change that adds gifts and not the split is a change that loses them.
