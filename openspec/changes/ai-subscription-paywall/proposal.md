## Why

Five of the six metered AI features (`tailor`, `match`, `assistant`, `dictation`,
`cover-letter`) still ship in shadow mode: `internal/ai/plan` counts a free account's daily
usage but never refuses it, so free access to AI tailoring, fit analysis, the assistant,
dictation and cover letters is today effectively unlimited. Only `auto-apply` already
refuses. The product decision is to make every metered AI feature subscription-only from
today, while keeping the existing per-feature env lever to reopen a free allowance later
without a deploy. Separately, the $19 Ultra tier is being pulled from sale for now (an
ops-side Stripe price-config change), and one UI surface still unconditionally advertises
it regardless of whether it is actually for sale.

## What Changes

- **BREAKING — the free-tier daily allowance for `tailor`, `match`, `assistant`,
  `dictation` and `cover-letter` becomes zero** (was 2, 3, 10, 10 and 3 respectively),
  matching the shape `auto-apply` already ships with.
- **Enforcement (`enforce: true`) turns on by default for those same five features**,
  ending shadow mode for them — a zero free allowance needs no shadow run to justify
  refusing it, unlike the earlier product numbers.
- `PLAN_FREE_DAILY_<FEATURE>` stays exactly as it is: the lever that reopens a free daily
  allowance for one feature via deploy config, with no code change. This is what "not
  removing the free plan, just not offering one yet" is built on.
- The account Plan page's "Upgrade to Ultra" call to action stops appearing for a Pro
  subscriber when no Ultra price is currently offered for sale. It already shows nothing
  to an Ultra subscriber, and shows the generic "Upgrade" (not Ultra-specific) call to
  action to a Free one.

## Capabilities

### New Capabilities

- `plan-tier-paywall`: the free tier's daily allowance is zero for every currently metered
  AI feature by default, refusal is enforced rather than shadowed, and any upgrade call to
  action on the account plan page only advertises a tier that is actually for sale.

### Modified Capabilities

(none — no existing `openspec/specs/` capability documents this behavior today; see
Impact)

## Impact

- `internal/ai/plan/plan.go` — `DefaultConfig()`'s `free`/`enforce` fields for the five
  features, and the doc comments above it and above `FeatureCoverLetter` that explain why
  they used to ship in shadow.
- `internal/ai/plan/AGENTS.md` — the "shadow run first" bullets that name `auto-apply` as
  the one exception.
- Root `AGENTS.md`'s "Plan limits" convention line, which currently reads "a plan differs
  in how MUCH ... never in whether the feature exists" as an unqualified rule.
- `internal/ai/plan`'s own test suite (`plan_test.go`, `decide_test.go`, `env_test.go`,
  `tier_test.go`, `session_test.go`, `store_integration_test.go`) — several tests assert
  the old default numbers or rely on `DefaultConfig()` being non-enforcing to exercise
  shadow-mode behavior generically.
- `web/src/lib/components/PlanView.svelte` — the "Upgrade to Ultra" CTA needs to read
  whether Ultra is currently offered (via the existing public `/api/v1/plans` prices list)
  before showing itself to a Pro subscriber.
- No change to `internal/api/handler` — every handler already calls `Consume`/`Standing`/
  `Refuses` per feature and already renders the 402 and `enforced` wire field; no handler
  code needs to change for the allowance-only part of this change.
- Out of scope: actually pulling Ultra from sale (unsetting `STRIPE_ULTRA_PRICE_IDS` on the
  production host) is an operational deploy action, not a code change, and is not part of
  this change.
