## Why

`add-pro-subscription` gave the product one way to buy Pro: a card, on the web, through
Stripe. That way does not exist on a phone. Apple's guideline 3.1.1 and Google Play's
Payments policy both require digital content consumed inside an app to be sold through the
platform's own in-app purchase, and neither store will approve a build that sends the user
to a web checkout for it instead. `freehire-mobile` therefore cannot sell anything today,
and the mobile client is where a candidate actually spends their commute.

The obstacle is not the store SDK — that lives in the mobile repository. It is that
`users.pro_until` was built for exactly one writer. `SyncUser` derives a timestamp from
Stripe's view of a customer and writes the column outright
(`internal/identity/billing/service.go:243`). Point a second provider at the same column and
the two erase each other: the first Stripe sync after an App Store purchase reads "this
account has no Stripe subscription", derives the zero time, and revokes a subscription the
user paid Apple for. Silently, and in the direction of taking away what was bought.

## What Changes

- **A second billing provider is added: RevenueCat**, which fronts App Store and Google
  Play purchases. Stripe keeps the web. Neither is retired and neither becomes primary.
- **`users.pro_until` becomes derived, not written.** Three source columns —
  `pro_until_stripe`, `pro_until_revenuecat`, `pro_until_granted` — carry each origin's
  reach, and `pro_until` becomes a stored generated column holding `GREATEST` of them. Every
  reader keeps reading the same column name with the same meaning; the hot path is
  untouched. A provider that writes only its own column cannot revoke another's grant,
  because the statement that would do it cannot be written.
- **The manual lane becomes explicit.** `pro_until_granted` is what support sets by hand and
  what `add-invites` will award. Until now a hand-set value lived in the same column a
  provider sync overwrites, so the next webhook silently undid it.
- **BREAKING — `users.pro_until` is no longer writable.** `UPDATE users SET pro_until = …`
  now errors. `SetProUntil` is replaced by per-source writers. This is the point of the
  change, not a side effect.
- **The billing package grows a provider seam.** One package, two providers: verify a
  delivery's signature, read a subscriber's current state. The existing Stripe code becomes
  one implementation; RevenueCat becomes the other. The rule that survives verbatim is the
  one worth keeping — a webhook is a signal that something changed, never a fact about what
  it changed to, so the handler re-reads the subscriber and derives the column from that.
- **`POST /api/v1/billing/revenuecat/webhook`** accepts deliveries, authenticated by the
  HMAC-SHA256 signature RevenueCat sends in `X-RevenueCat-Webhook-Signature`.
- **`POST /api/v1/billing/revenuecat/sync`** lets an authenticated caller ask for their own
  state to be re-read now. A store purchase completes on the device seconds before the
  webhook lands, and a paywall that says "wait" after a successful payment is a support
  ticket. It doubles as the recovery path when a delivery is lost, which is the same job the
  reconciler does on an hourly schedule.
- **`GET /api/v1/me/plan` gains `pro_source`** — which origin currently confers Pro. Without
  it a client cannot tell an App Store subscriber from a Stripe one, and the two must be
  told apart: Apple forbids sending an in-app subscriber to a web page to cancel, and
  offering an in-app purchase to someone already paying through Stripe sells them the same
  thing twice.
- **`cmd/billing-sync` reconciles both providers**, each against its own column.
- **RevenueCat is off unless configured**, exactly as Stripe is. Absent `REVENUECAT_*` the
  routes are not mounted and the worker skips that provider.

## Capabilities

### New Capabilities

- `store-purchases`: how a purchase made in the App Store or Google Play reaches
  `users.pro_until`, how two providers coexist without erasing each other, what
  `pro_source` means and what a client may do with it, how the synchronous sync route
  behaves and how it is bounded, and how the whole surface degrades when RevenueCat is not
  configured.

### Modified Capabilities

None, and the absence is deliberate rather than a claim that nothing changed.

Two requirements in `pro-subscription` are amended by this change: "The plan column has
exactly two writers" — there are now more, and they write source columns — and "The plan
column is derived from the provider's current state", which now describes one source among
three. Neither can carry a delta spec, because `pro-subscription` is not in
`openspec/specs/`: `add-pro-subscription` is still unarchived. The amended rules are stated
inside `store-purchases` instead, naming what they supersede, and `add-pro-subscription`'s
own proposal set this precedent for the same reason against `plan-limits`.

## Impact

- **Schema:** one migration adding three columns to `users`, splitting existing values by
  origin, and replacing `pro_until` with a generated column. It rewrites the table, which at
  the measured 1397 rows on prod (`migrations/0129_users_stripe_customer_id.sql`) is
  milliseconds. It must still be run manually before the deploy, because the ordering matters
  even though the duration does not.
- **Go:** `internal/identity/billing` gains a provider seam and a RevenueCat
  implementation, both inside the existing package — no new entry in
  `internal/platform/arch/layering/blocks.go`. `SetProUntil` is replaced. `cmd/billing-sync`
  gains a second pass. New routes under `/api/v1/billing/revenuecat`.
- **HTTP:** `GET /me/plan` gains one field. No existing field changes shape.
- **Ops:** three new environment variables in `/opt/freehire/.env`. Creating the RevenueCat
  project, its entitlement and offering, and registering the webhook with HMAC signing are
  manual dashboard steps, recorded in `deploy/`.
- **Not in this change:** the mobile client itself, which is a separate change in
  `freehire-mobile` and depends on the contract this one establishes; a paywall or pricing
  surface of our own on the web; migrating Stripe subscriptions into RevenueCat; gifted Pro
  days, which is `add-invites` and only needs the column this change creates; and refunds
  or cancellation from our UI, which stay with whichever store or portal sold the
  subscription.
