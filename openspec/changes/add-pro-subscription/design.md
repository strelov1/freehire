# Design — add-pro-subscription

## Why this code is public

The question that opened this change was whether billing belongs in the open repository
at all. It does, and the reasoning is worth recording because the instinct to close it is
strong and the usual arguments for closing it do not apply here.

**The two stated worries are not addressed by closing the code.** Secrets never live in
source, open or closed: they live in `/opt/freehire/.env`, and `gitleaks` guards both the
staged index and the whole history. Legal exposure — who is the seller, who owes VAT, who
refunds — is decided by the legal entity and the payment provider, not by the visibility
of a webhook handler.

**The projects that do close their billing are answering a different question.** GitLab,
Sentry and Cal.com sell *software* that a competitor could self-host commercially, so
they need a licence boundary; billing merely happens to sit on the far side of it. Ghost
and Plausible sell a *hosted service* and ship their Stripe and Paddle integrations in
the open. freehire is the second kind, in an extreme form: the moat is 8M postings, the
crawl fleet and the production host, none of which is reproduced by reading this package.
And the boundary is in any case already given away — the repository is MIT with 105
forks, so an `ee/`-style carve-out would not retroactively restrict the existing code.

**The one real cost is noise, and it is bought off cheaply.** Open billing attracts
issues and pull requests from self-hosters. The mitigation is a single package with an
`AGENTS.md` stating that this is freehire.me's hosted billing and is unsupported for
self-hosting, plus the configuration rule below that makes it invisible to anyone who has
not deliberately turned it on. That buys the quiet other projects pay for with a second
repository, without paying the price of two repositories owning one database.

**Rejected: a private billing service.** It would need to write `users.pro_until`, so two
repositories would own one schema while `migrations/`, `make sqlc` and
`internal/platform/arch/layering/blocks.go` all stay here. A permanent seam across every
deploy, in exchange for protection that MIT already surrendered.

## Ownership of `users.pro_until`

`migrations/0120_users_pro_until.sql` already states the shape: the provider's record —
product, status, period, payment method — stays with the provider, and we keep one
derived timestamp. This change names the writers.

After this change `users.pro_until` is written by exactly two callers, both in
`internal/identity/billing`: the webhook handler and `cmd/billing-sync`. Nothing else
writes it. A hand-set value in psql remains safe for support, because a timestamp expires
by itself and forgetting to remove one costs nothing — the property 0120 was chosen for.

The value written is always **derived from RevenueCat's current subscriber state**, never
accumulated from events. That is what makes the write idempotent, order-independent and
safe to repeat: applying the same truth twice changes nothing.

## The webhook is a signal, not a fact

The obvious implementation reads `event.type` and moves a local state machine:
`INITIAL_PURCHASE` grants, `CANCELLATION` revokes, `RENEWAL` extends. It is wrong for two
independent reasons.

**Ordering.** RevenueCat does not guarantee delivery order. A `RENEWAL` arriving after the
`EXPIRATION` it superseded would leave the account wrong until someone noticed.

**Divergence.** Branching on event types builds a copy of the provider's state machine
here, and every event type they add or re-interpret — transfers between accounts, billing
retries, grace periods, refunds — is a way for the copy to drift from the original. The
copy would be a second source of truth about money.

So the handler treats an event as "something about this user changed" and answers it by
re-reading: `GET /v1/subscribers/{app_user_id}`, take the latest `expires_date` across
the entitlements that count as Pro, write it. Refunds, transfers and grace periods need
no code of their own, because they are already reflected in what we re-read.

The cost is one outbound call per event. At the volume a $5 subscription reaches, this is
not a consideration.

## `billing_events`, and why a table earns its place

The webhook writes the event and returns 200 **before** attempting to apply it. One
append-only table then does three jobs that would otherwise need three mechanisms:

- **Idempotency.** `UNIQUE (provider, event_id)` makes a redelivered webhook a no-op.
  RevenueCat retries, so redelivery is normal, not exceptional.
- **Retry.** An event that could not be applied — RevenueCat unreachable, our pool
  saturated — stays unprocessed and is picked up by `cmd/billing-sync`. The webhook
  therefore never has to fail to be safe.
- **Audit.** "Why is this account Pro?" is a question about money, and it needs an answer
  in writing. The stored payload also makes a mapping bug replayable rather than
  archaeological.

Returning 200 before applying is deliberate: a 500 would make RevenueCat retry something
we have already durably recorded, and the retry would race the reconciler for the same
work. Once the event is stored, delivery has succeeded.

After storing, the handler tries to apply inline with a short timeout. This is purely for
latency — a candidate who has just paid should see Pro immediately, not at the top of the
next hour. Failure is not an error path; it is the normal case falling back to the worker.

## `cmd/billing-sync`

A `Type=oneshot` cron worker, hourly, in the same mould as the rest of `cmd/`. It:

1. applies `billing_events` rows with `processed_at IS NULL`;
2. re-reads the subscriber state for users whose `pro_until` sits within a window around
   now, catching a renewal whose webhook never arrived.

Both passes start from `billing_events`, never from `users`: the second joins the event
table to `users` and filters on `pro_until`, so the candidate set is the subscriber base
rather than an 8M-row scan, and no index on `users` has to be built to support it. It also
makes the "never ask the provider about a stranger" rule structural — the provider's GET
creates a subscriber for an unknown id, and a query that can only reach accounts which
have transacted cannot trip that.

**A user who has never produced a stored event is invisible to the worker**,
and this is an accepted limit rather than an oversight: a purchase always produces an
event, we store events before processing them, and RevenueCat retries undelivered
webhooks for days. A purchase that produces no event at all within that window is a
provider outage, and the recovery is a support action, not a scheduled scan. Listing all
customers from the provider was considered and rejected as a scan of the entire
subscriber base, every hour, to find a case that has not happened.

## Placement

`internal/identity/billing`, block `identity`, layer 3. A subscription is an attribute of
the account, which is what that block holds.

The obvious worry is the one that put `internal/ai/plan` in `ai` instead: `identity` and
`ai` share layer 3, so nothing in `ai` can import `identity`. It does not apply here.
`plan` reads `pro_until` through `platform/db` and does not import `billing`; the callers
of `billing` are the webhook handler in `api` (layer 8) and a binary in `cmd/`, both of
which may import anything. The rule is satisfied and the code is possible.

## Checkout

The purchase happens on RevenueCat's hosted Web Billing paywall. We return a URL ending in
the user's identifier as a path segment; we never handle a card, hold a Stripe credential
or render a payment form.

**The URL is built server-side, by a handler**, not assembled in the SPA from a public
environment variable. The identifier in it decides who gets charged and who becomes Pro,
and a value the browser composes is a value the browser can alter.

**`app_user_id` is always our `users.id`, and only after authentication.** RevenueCat
assigns an anonymous identifier to a client that has not been identified, and a purchase
made under one lands on an account we cannot resolve. The checkout route requires auth,
which makes the mistake unreachable rather than merely documented.

A pricing page of our own was considered and deferred. It converts better, and it is
frontend work with more places to break; the hosted paywall is the smaller thing that
ships and can be replaced without touching anything else here.

## Off unless configured

Absent `REVENUECAT_API_KEY` and `REVENUECAT_WEBHOOK_TOKEN`, every route under
`/api/v1/billing` returns 404 and `cmd/billing-sync` exits 0 without opening the pool.
Not 500, and not a startup failure: an unconfigured optional subsystem degrades quietly
in this fleet, and a self-hoster who never sets these variables should not be able to
tell that the code is there.

The signature comparison is constant-time (`crypto/subtle`). It is the whole of the
authentication on this endpoint, so a timing-variable compare is the only way it can leak
anything.

## Seams noted, not built

**Gifted Pro days.** `add-invites` grants days of Pro, and if it writes `pro_until` the
reconciler will overwrite gifts with the provider's truth. The resolution is a separate
column for granted days with `pro_until` derived as the later of the two — but there are
no gifts yet, and building the split now is infrastructure ahead of need. `add-invites`
owns it, and this note is the handover.

**Merchant of record.** RevenueCat is not one. Purchases through the App Store make Apple
the seller; purchases through Web Billing leave us the seller, and the VAT and sales-tax
obligations that come with it, in every jurisdiction a customer is in. Web is where most
freehire users are. This does not change any code in this change, and it is not a
software decision — recorded here because the alternative (Paddle or Lemon Squeezy as
merchant of record, roughly 5% against roughly 3%) is chosen at the provider level and
would be a straight replacement of this package's one outbound client.

**A provider port.** There is one provider and there is no interface for it. A second
provider would introduce one; inventing it now would be a guess at the shape of a
provider we have not read the documentation of.

## The provider contract, as verified

Read from RevenueCat's documentation on **2026-09-03**. Everything below is what the
implementation is built against; re-read it if the client starts failing in a way the
tests do not reproduce.

**Webhook envelope.** `{"api_version": "1.0", "event": {…}}`. The event carries `id`
(**retries reuse the same id**, which is what makes `UNIQUE (provider, event_id)` the
correct idempotency key), `app_user_id`, `type`, `expiration_at_ms` and `entitlement_ids`.

**Delivery.** Explicitly **not ordered and not strictly guaranteed**, with duplicates
possible — the documentation itself tells implementers to deduplicate on the event id.
This is the design's premise, confirmed rather than assumed. Non-200 triggers **five**
retries at 5, 10, 20, 40 and 80 minutes, and then delivery **stops for good**. The
response budget is 60 seconds, and the documentation advises acknowledging first and
deferring the work.

That retry ceiling is the strongest argument in this document for `cmd/billing-sync`:
about two and a half hours after a purchase, RevenueCat stops trying, and the reconciler
is the only remaining path by which a paid subscription becomes Pro.

**Authentication — HMAC, not the shared header.** RevenueCat can sign each delivery:
`X-RevenueCat-Webhook-Signature: t=<unix>,v1=<hex>`, an HMAC-SHA256 over
`"<t>.<raw body>"` with the integration's signing secret. This supersedes the shared
`Authorization` value the change originally specified, which is a bearer token replayable
by anyone who ever sees one delivery. We prefer the signature and fall back to the header
only if the dashboard turns out not to offer signing on this integration.

Two properties are load-bearing. **The HMAC covers the raw bytes**, so it must be computed
over the body as received — a parse-and-reserialise changes the bytes and rejects valid
deliveries. And **`t` must be checked for freshness**, because a signature with no
time bound is a replayable credential like the header it replaces.

**Checkout URL.** `https://pay.rev.cat/<token>/<app_user_id>` — the identifier is a **path
segment, not a query parameter**, and must be URL-encoded. `BILLING_CHECKOUT_URL` holds
everything up to the token; the handler appends the segment. A custom post-purchase
redirect back to our own site is supported, and `package_id` preselects the product.

**Subscriber state.** `GET https://api.revenuecat.com/v1/subscribers/{app_user_id}` with
`Authorization: Bearer sk_…` — a **secret** key, server-side only. Entitlements arrive as
a map keyed by entitlement id, each with `expires_date`, `grace_period_expires_date`,
`product_identifier` and `purchase_date`, in ISO 8601. The response also carries
`management_url`, which is where a subscriber cancels — that is the destination the
delete-account surface must name, rather than a URL of our own invention.

**Two traps in that endpoint.**

`expires_date` is **nullable**, and null means a non-expiring entitlement rather than an
expired one. Reading null as zero would silently downgrade a lifetime purchaser to free.
We do not sell a lifetime product, which makes this a case the code must handle
*correctly* precisely because nobody will notice it being handled wrongly.

And the GET **creates the subscriber if the id is unknown** — a read with a write's
consequences. It is only ever called for a user who already has a recorded event or a
non-NULL `pro_until`, so we never manufacture subscribers; the constraint is now a
property the reconciler must preserve, not merely a habit.
