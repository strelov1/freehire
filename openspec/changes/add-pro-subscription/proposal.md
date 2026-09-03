## Why

`add-plan-limits` shipped the whole of the paywall except the part that takes money.
`users.pro_until` decides every allowance in the product, and today the only way to set it
is by hand in psql. There is a Pro plan, there is a 402 that tells a candidate where to
upgrade, and there is nowhere to upgrade.

The shape of the missing piece was fixed when that column was added: the billing provider
holds the subscription, we hold one derived timestamp, and the metering path reads the
column rather than an API. What is left is a webhook, a reconciler and a link to a
checkout page — deliberately small, because the column was designed to keep it small.

This change also settles a question that was open: **the billing code lives in this
public repository under the same MIT licence as everything else.** Closing it was
considered and rejected — see the design.

## What Changes

- **A candidate can buy Pro.** An authenticated user gets a checkout URL for
  RevenueCat's hosted Web Billing paywall, carrying their `users.id` as `app_user_id`.
  We never see a card number and hold no Stripe credential.
- **`users.pro_until` is written by machine, not by hand.** A RevenueCat webhook and an
  hourly reconciler are the only writers.
- **A webhook is treated as a signal, never as a fact.** The handler does not branch on
  the event type to decide entitlement; it records the event, then re-reads the
  subscriber's state from RevenueCat and derives `pro_until` from that. RevenueCat does
  not guarantee webhook ordering, and a locally-maintained copy of a provider's state
  machine is a second source of truth that will disagree with the first.
- **`billing_events` is added** — an append-only record of every event received, unique
  on `(provider, event_id)`. It is the idempotency key, the retry queue and the audit
  trail for "why is this account Pro", which is a question about money and therefore
  needs a written answer.
- **`cmd/billing-sync` is added** — an hourly, run-once-and-exit worker that applies
  events the webhook could not and re-checks subscriptions near their expiry. Webhook
  delivery is best-effort; a paid subscription that silently lapses is the failure this
  worker exists to prevent.
- **Billing is off unless configured.** Without `REVENUECAT_*` in the environment the
  routes 404 and the worker is a no-op that never opens the pool — the same degradation
  `cmd/queue-metrics` has without `PROM_TEXTFILE_DIR`. This is what makes the code safe
  to publish and harmless to a self-hoster.
- **The 402 body's upgrade pointer becomes a real destination.** The SPA gains a minimal
  upgrade entry point; the paywall page itself is RevenueCat's, not ours.

## Capabilities

### New Capabilities

- `pro-subscription`: how a Pro subscription is bought, how the provider's state reaches
  `users.pro_until`, what happens when a webhook is lost, duplicated or delivered out of
  order, and how the whole surface behaves when billing is not configured.

### Modified Capabilities

None. Two rules that read as amendments elsewhere are stated inside `pro-subscription`
instead, deliberately: the ownership of `users.pro_until`, because `openspec/specs/plan-limits/`
does not exist yet — `add-plan-limits` is still an unarchived change — and the erasure of
billing records, because the two `account-deletion` requirements that would carry it are
already modified by `add-plan-limits`, and a second change editing the same requirements
would collide on sync.

## Impact

- **Schema:** one new table, `billing_events`. `users.pro_until` is unchanged.
- **Go:** new package `internal/identity/billing`, registered in
  `internal/platform/arch/layering/blocks.go`. New binary `cmd/billing-sync`. New handler
  routes under `/api/v1/billing`.
- **SPA:** an upgrade entry point and a "you are on Pro until …" line on the usage
  surface. No pricing page of our own.
- **Ops:** four new environment variables, all secrets in `/opt/freehire/.env`; one new
  systemd timer. Registering the webhook, enabling its HMAC signing and creating the
  paywall are manual dashboard steps, recorded in `deploy/`.
- **Not in this change:** invites and gifted Pro days (`add-invites`); cancelling or
  refunding from our UI (RevenueCat's customer portal owns that); a pricing page of our
  own design; annual plans; and reducing the $0.48 assistant turn, which is what actually
  decides whether $5/month is profitable.
