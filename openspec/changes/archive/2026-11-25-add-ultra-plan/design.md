## Context

Two plans exist because the code can only represent two. `plan.TierOf` reads
`users.pro_until` — a timestamp — and answers pro or free. There is no record anywhere of
WHAT an account bought, only of how long it lasts, so a third tier is not a configuration
change but a modelling one.

The feature Ultra is meant to sell is not metered at all. `PostJobAutoApply` admits anybody
on pro and counts nothing, so the heaviest action the product runs — a headless browser
filling and submitting a real application — is flat-rate at $5.

Migration 0135 is the shape to copy. It split `pro_until` into three source columns, one per
origin, each with exactly one writer, and derived the effective value as the furthest of
them. That exists because a provider reporting "no subscription" is saying *I confer
nothing*, never *this account is not pro* — and before 0135 the difference silently revoked
manual grants.

## Goals / Non-Goals

**Goals:**

- A third tier that the system can represent, resolve and sell.
- auto-apply measured, so the tier above can be priced on it.
- One place that says what each tier allows per feature per day.
- Nothing in the source that knows a price id.

**Non-Goals:**

- An Ultra product in the App Store or Google Play. The column is created; the product is
  not.
- A plan-switching flow of our own. The provider's portal already changes a subscription,
  and `billing/AGENTS.md` rejects writing our own cancellation flow for the same reason: it
  would be one more thing that can disagree with what happened to the money.
- Proration, refunds, or upgrade credit. The provider owns those.
- Metering anything else. The five existing features keep their numbers and their shadow
  mode.

## Decisions

### Ultra gets its own entitlement columns, not a `tier` column

`ultra_until_stripe`, `ultra_until_revenuecat`, `ultra_until_granted`, and a derived
`ultra_until` that refuses assignment — migration 0135's shape, applied again.

*Alternative considered:* one `users.plan_tier` column. Rejected, and the reason is the one
0135 was written for. A single column has a single writer, so two providers and a support
grant would have to agree on who last wrote it; a provider that reports "no Ultra
subscription" would then either clear a tier some other origin still confers, or be unable
to clear its own. Splitting by origin is what makes "I confer nothing" expressible, and the
second tier does not get to skip the lesson the first one paid for.

The columns also mean the SHAPE of the answer stays a date. Everything downstream — the
reconciler's near-expiry sweep, the plan read on every metered action — already works in
instants, and would have needed a second mechanism for a tier that worked in names.

### The provider seam carries a value per tier

`reach` currently returns one `time.Time` and `store` writes one column. Both widen to carry
a small per-tier value:

```go
// billing
type entitlement struct {
    Pro   time.Time
    Ultra time.Time
}
```

**One `reach` returning both, not two calls.** A webhook is a signal and the provider is
re-read whole (`billing/AGENTS.md`); two reads could land either side of a change and
disagree with each other, which is exactly the "derived whole, never adjusted" rule this
package is built on. One read, both answers, both columns written together.

**A provider writes every tier column it owns, including the ones it reports nothing for.**
RevenueCat sells no Ultra today and returns a zero — and still writes it. A provider that
only wrote the columns it had something to say about could never take back what it once
said, which is the cancellation path.

### Which tier a price confers is a list, and the machinery exists

`entitlement.go`'s `proUntilFrom(sub, proPrices)` already filters a subscriber's
subscriptions to those whose price appears in a configured list. A second list —
`STRIPE_ULTRA_PRICE_IDS` — is the whole of it. No price id ships in the source, exactly as
today, and a deployment that names no Ultra prices simply never resolves anybody to ultra.

*Alternative considered:* metadata on the Stripe price. Rejected — it puts the tier
definition in the provider's dashboard, where nothing in this repository can test it and a
mis-click silently upgrades everybody.

### `featureConfig` is reshaped to one allowance per tier

Today it holds `freeDaily` plus `proFairUse`, because pro was unlimited everywhere and free
was the only tier with a real number. Ultra breaks that: **pro's auto-apply is a hard 3 a
day, not unlimited-with-fair-use.** Bolting a `proDaily` beside `proFairUse` would leave two
fields whose interaction nobody can read.

So the configuration carries an `Allowance` per tier, and the fair-use figure attaches to
the tiers whose allowance is unlimited. `Allowance(tier, feature)` becomes a lookup rather
than an `if tier == TierPro`. `decide.go`'s fair-use branch generalises the same way —
`c.FairUse(tier, f)` instead of `c.ProFairUse(f)`.

This is the reshape `AGENTS.md` asks for over an awkward special case: the current structure
is not load-bearing legacy, and a third tier is exactly the new feature that does not fit
cleanly into it.

Environment knobs follow the existing naming: `PLAN_FREE_DAILY_<F>`, `PLAN_PRO_DAILY_<F>` and
`PLAN_ULTRA_DAILY_<F>`, one per tier. The pro auto-apply number in particular must move
without a deploy, because it is a ceiling under a plan people already bought.

**Amended during implementation.** This section originally kept `PLAN_FAIR_USE_<F>` alongside
`PLAN_PRO_DAILY_<F>`. Both wrote the same field — a tier's figure is its ceiling where it has
one and its fair-use guard where it does not — so keeping both meant two names for one thing,
which the same paragraph argues against elsewhere. Nothing had ever set the older name (no
`PLAN_*` variable is set on the production host at all), so it was removed rather than
carried. Recorded here rather than left as a silent deviation from the spec.

### A refusal offers an upgrade to anybody who is not on the top tier

Added during implementation, and it reaches all six features rather than auto-apply alone.

The 402 offered its upgrade link to the free tier only, which was correct while pro was the
top of the range: there was nothing to sell a subscriber. With Ultra above it that rule
withholds the link from exactly the person it is worth something to — a pro subscriber
refused for auto-apply — and the refusal is the moment it is worth it. Written as "not the
top tier" so that adding a fourth does not silently stop offering it to the one it displaced.

Wider than the spec asked for, and deliberately: leaving the other five features on the old
rule would mean two upgrade rules differing by feature, which is a thing to discover rather
than a thing to read.

### The account plan surface reads the tier the caller is on

`GET /me/plan` filled `pro_until` and `pro_source` from the pro columns unconditionally, so
an ultra subscriber was answered `plan: "ultra"` with neither. `pro_source` is behavioural —
it exists so a client does not offer an in-app purchase to somebody already paying through
Stripe — so an empty one on a paying account is the exact double-charge the field prevents,
produced by the endpoint meant to prevent it. The fields keep their `pro_` spelling, which
clients read; what they have always meant is "when does the plan you are paying for run out,
and where did you buy it".

### Tier resolution is Ultra > Pro > Free

Highest wins. Somebody holding both subscriptions — which the portal makes possible during
an upgrade — must not be given less than either one alone, and a plan resolution that could
go down when a customer spends more is a bug people notice in the worst possible way.

### auto-apply is charged before the queue write, keyed by the posting

`Consume(user, feature, ref)` is idempotent by `ref`. With `ref` set to the posting's id, the
ordering question answers itself:

- A double-clicked button charges once — the second call reads as already charged.
- A retry after a database error charges once, for the same reason.
- An over-allowance request is refused before anything is queued.

Charging *after* the queue write would need the queue row's own id as the reference, which a
duplicate request never gets — so the second click would charge again. Charging before,
keyed by the posting, gets idempotency from the key rather than fighting it, which is the
same trick `sessionRef` uses for tailoring turns.

The checks that are not about allowance — an unsupported platform, no CV, already applied —
stay ahead of the charge, so a request refused for those spends nothing.

### This one feature enforces from the first deploy

Every other metered feature ships with `enforce: false` and is watched in shadow first. That
convention exists to protect people from ceilings nobody has verified against real
behaviour. It cannot apply here: a pro ceiling that only counts leaves Ultra selling
nothing, and shipping the tier before the ceiling means selling a plan that is not yet
different.

It is also less of a break than it looks. `PostJobAutoApply` already refuses with 402 today
— for the whole free tier. The allowance replaces that gate rather than adding a new kind of
refusal, and the pre-check idiom `plan/AGENTS.md` mandates (`Refuses`, never `Exhausted`)
keeps every surface honest about it.

## Risks / Trade-offs

- **Existing pro subscribers lose unlimited auto-apply.** → Two accounts, and two queue
  entries in the feature's entire history. The number is an environment variable, so if
  three a day turns out wrong it moves without a deploy. Doing this later means doing it to
  more people.

- **`TierOf` changes signature, and it is read on every metered action.** → The compiler
  finds every caller; the risk is a caller that passes the same instant twice and silently
  resolves everybody to ultra. A test asserts that an account with only a pro entitlement
  resolves to pro through the real store.

- **Enforcement on from day one means a wrong number refuses real people.** → It refuses
  with the figures attached, and the figure is configurable. The alternative — shipping a
  tier that sells nothing — is worse and harder to explain.

- **A deployment that forgets `STRIPE_ULTRA_PRICE_IDS` sells no Ultra silently.** → It also
  changes nothing else: pro and free behave exactly as before, and the pricing page shows
  what the API offers. An absent price list is the same "disabled" shape the whole billing
  package already uses.

- **RevenueCat writes an Ultra column for a product it does not sell.** → Deliberate. The
  cost is a column of nulls; the alternative is a provider that cannot clear what it never
  wrote.

## Migration Plan

1. Migration `0141_ultra_until_sources.sql` adds the three source columns and the derived
   `ultra_until`. Additive; `make sqlc` regenerates the queries.
2. Deploy. With no Ultra price configured, nobody resolves to ultra and the only visible
   change is auto-apply's pro ceiling.
3. Create the Ultra price in Stripe by hand and name it in `STRIPE_ULTRA_PRICE_IDS` on the
   host. The pricing page picks it up.
4. **Rollback** is unsetting `STRIPE_ULTRA_PRICE_IDS` — nobody new resolves to ultra, and
   anybody already on it keeps what they paid for until it lapses. Reverting the auto-apply
   ceiling is `PLAN_PRO_DAILY_AUTO_APPLY` set high, also without a deploy.

## Open Questions

- **Should an Ultra subscriber's auto-apply carry a fair-use guard at all?** Pro's guard sits
  twenty times above human behaviour and protects the gateway rather than a price. auto-apply
  costs a browser rather than a model call, so the number that protects the host is a
  different one and there is no measurement yet. Shipping with a deliberately loose figure
  and reading it after a month is the plan.
- **What happens to somebody who upgrades mid-period?** The portal prorates and both
  subscriptions can be live for an instant, which the Ultra-wins rule already covers. Whether
  we should also cancel the pro subscription for them is a support question, not a code one,
  until it happens twice.
