## Why

There is one thing to buy. A candidate who has outgrown Pro's daily allowances has nowhere
to go, and the product's most expensive action — auto-apply, which drives a headless browser
and submits a real application to a real employer — is the one thing nobody meters at all.

Both halves of that are the same problem. `internal/api/handler/auto_apply_enqueue.go`
admits anybody on Pro and counts nothing, so the heaviest feature we run is sold flat at $5
and the account that uses it a hundred times a day pays what the account that uses it twice
pays. A second tier priced on that action is only possible once the action is measured.

Now is when this costs least. Production carries **two** paying subscribers and **two**
auto-apply entries in the queue's whole history. Putting a ceiling under Pro's auto-apply is
a change to a plan somebody bought — which is a conversation with two people today, and an
incident in six months.

## What Changes

- **A third tier, Ultra, at $19 a month.** Unlimited auto-apply and roughly triple Pro's
  daily allowance on every AI feature.
- **auto-apply becomes a metered feature** — the sixth. Free 0 (unchanged: it is a paid
  feature today and stays one), Pro 3 a day, Ultra unbounded. The handler's own
  `!= TierPro` check is replaced by the allowance, so "this is a PRO feature" becomes an
  ordinary refusal carrying the numbers behind it.
- **BREAKING for existing Pro subscribers**: auto-apply was unbounded for them and is now 3
  a day.
- **The tier gets somewhere to live.** `plan.TierOf` reads `users.pro_until`, which is a
  DATE and not a record of what was bought — the system cannot currently represent a third
  tier at all. New source columns `ultra_until_stripe`, `ultra_until_revenuecat`,
  `ultra_until_granted` and a derived `ultra_until`, mirroring migration 0135 exactly.
- **The provider seam widens from one instant to two.** `reach` returns how far a provider's
  entitlement reaches; with two tiers it has to answer for both, and `store` has to write
  both columns. Stripe tells them apart by price list, which is machinery that already
  exists — a second list, `STRIPE_ULTRA_PRICE_IDS`, is all it needs.
- **Enforcement is ON for auto-apply from the first deploy**, unlike every other metered
  feature, which ships counting-only behind `PLAN_ENFORCE`. A ceiling nobody enforces would
  leave Ultra selling nothing. It is not a break with the convention either: that route
  already hard-refuses with 402 today, and the allowance takes that gate's place.
- **The pricing page grows a third column**, reading its numbers from the API as it does
  now.

## Capabilities

### New Capabilities

- `plan-tiers`: What tiers exist, how one is resolved from what an account holds, and what
  each allows per feature per day.
- `auto-apply-allowance`: auto-apply as a metered feature — who may start one, how many a
  day, and when the allowance is actually spent.

### Modified Capabilities

None. Neither the plan limits nor the Pro subscription has a spec in `openspec/specs/` yet
(`add-plan-limits` and `add-pro-subscription` are still unarchived changes), so there is no
existing requirement to restate.

## Impact

- **Schema**: migration `0141_ultra_until_sources.sql` — three source columns plus a derived
  `ultra_until` that refuses assignment, exactly as `pro_until` does. Additive; nothing
  applied is edited. `make sqlc` regenerates `internal/platform/db`.
- **Go — `internal/ai/plan`**: `TierUltra`; `TierOf` takes both instants and resolves
  Ultra > Pro > Free; `featureConfig` gains an Ultra allowance; `FeatureAutoApply` joins the
  five existing features. `decide.go`, `session.go` and `store.go` follow.
- **Go — `internal/identity/billing`**: `reach` and `store` carry a per-tier value rather
  than one instant; `entitlement.go` resolves each tier from its own price list;
  `config.go` reads the second list. RevenueCat answers zero for Ultra — there is no store
  product — but writes its column, because a provider that could not write it could not
  clear it either.
- **Go — `internal/api/handler`**: `PostJobAutoApply` consumes an allowance instead of
  checking a tier, and charges only when a queue row is actually created.
- **Web**: `/pricing` renders three plans; `PlansMatrix` in `web/src/lib/types.ts` carries a
  third number per feature.
- **Environment**: `STRIPE_ULTRA_PRICE_IDS` on the host. Absent, Ultra is simply not sold and
  everything else behaves as it does today.
- **Operations**: the Ultra price is created in Stripe by hand, like the Pro one, and named
  in the host environment. No code ships knowing a price id.
