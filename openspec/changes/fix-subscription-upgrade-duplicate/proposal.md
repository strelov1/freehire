## Why

Upgrading from Pro to Ultra (and any other tier-to-tier change) opens a brand-new Stripe Checkout Session instead of changing the customer's existing subscription. The old subscription is never cancelled, so the customer ends up with two independent, concurrently-billing Stripe subscriptions (e.g. Pro $5/mo + Ultra $19/mo), and the app's own billing overview shows only the better-entitling one, hiding the still-active duplicate charge from the customer. A real customer was double-billed this way (freehire#none yet — found via support report on 2026-09-09).

## What Changes

- `POST /api/v1/billing/checkout` (or a new endpoint) modifies an existing entitling subscription in place — via Stripe's subscription-item replacement with proration — when the requesting customer already has one, instead of opening a second Checkout Session. Checkout Session creation is used only for a customer's first subscription.
- The billing overview (`GET /billing/subscription`) reports when a customer has more than one entitling subscription active, instead of silently picking the best one, so a customer already in this state (or reached by a future bug) can see and act on it rather than being charged for both.
- Impacted files: `internal/api/handler/billing.go`, `internal/identity/billing/service.go`, `internal/identity/billing/client.go`, `internal/identity/billing/entitlement.go`, `internal/identity/billing/overview.go`, `web/src/lib/components/PlanView.svelte`, `web/src/routes/pricing/+page.svelte`, `web/src/lib/api.ts`.

## Capabilities

### New Capabilities
- `subscription-billing`: purchasing, upgrading/downgrading, and viewing the state of a paid subscription (Pro/Ultra) against the payment provider (Stripe) — the checkout flow, the upgrade-in-place behavior, and what the billing overview reports.

### Modified Capabilities
(none — `subscription-billing` did not exist as a spec before this change)

## Impact

- **Backend**: `internal/identity/billing` (service, client, entitlement, overview) and `internal/api/handler/billing.go`. No schema/migration change expected — Stripe is the system of record for subscription state, per `internal/identity/billing/AGENTS.md`'s "no local subscription state machine" rule.
- **Frontend**: `web/src/lib/components/PlanView.svelte`, `web/src/routes/pricing/+page.svelte`, `web/src/lib/api.ts` — the upgrade button's request/redirect flow, and a warning surfaced if the overview reports more than one entitling subscription.
- **External**: Stripe API — introduces the first call in this codebase to `POST /v1/subscriptions/{id}` (update with proration); no new webhook events needed, since `customer.subscription.updated` is already handled.
- **Manual remediation**: this change does not touch already-duplicated Stripe subscriptions for existing customers — an operator must cancel the extra subscription by hand (e.g. via the Stripe dashboard or Customer Portal) for anyone already affected.
