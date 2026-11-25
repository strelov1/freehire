## Context

`internal/ai/plan` already fully implements per-feature daily allowances, enforcement,
402 refusal, the `enforced` wire flag, and the SPA's `refuses()` gate (see
`internal/ai/plan/AGENTS.md`). Every handler for the five features in scope already calls
`Consume`/`Standing`/`Release` correctly. `FeatureAutoApply` already ships with
`free: tierAllowance{}` (zero) and `enforce: true` — this change makes the other five
features match that existing shape; it does not add a mechanism.

See proposal.md - Why for the product motivation.

## Goals / Non-Goals

**Goals:**
- Flip `DefaultConfig()`'s `free`/`enforce` fields for `tailor`, `match`, `assistant`,
  `dictation`, `cover-letter` to match `auto-apply`'s existing shape.
- Keep every existing lever (`PLAN_FREE_DAILY_<FEATURE>`, `PLAN_ENFORCE`) working exactly
  as documented, so a free allowance can be reopened later without a deploy.
- Fix the one UI surface (`PlanView.svelte`'s "Upgrade to Ultra" CTA) that hard-advertises
  a tier regardless of whether it is for sale.
- Keep the package's own test suite meaningful: several tests currently use `FeatureFit`
  via `DefaultConfig()` purely as a stand-in with "some positive free number" to exercise
  the generic `decide`/store mechanism (idempotency, day rollover, shadow behavior,
  boundary refusal) — not to assert the product's actual number. Zeroing the product
  default must not silently make those tests vacuous (a loop bound of `0` that never
  executes, quietly weakening coverage) or force them to assert a fact that is no longer
  true (`DefaultConfig()` being non-enforcing).

**Non-Goals:**
- Unsetting `STRIPE_ULTRA_PRICE_IDS` on the production host — an operational deploy
  action outside this repo's code, left to the user.
- Reconciling the stale, unsynced `openspec/specs/job-fit-analysis` (still describes the
  pre-`plan` rolling-30-day quota) or archiving `add-plan-limits`/`add-ultra-plan` — a
  pre-existing documentation gap unrelated to this change's behavior.
- Any change to how `pro`/`ultra` daily numbers are computed, or to the fair-use guard.
- Any handler or wire-format change — the existing `Consume`/`Standing`/`Refuses` plumbing
  and the `enforced` field on the allowance response already do everything a paywall needs.

## Decisions

**Hardcode the new default in `DefaultConfig()`, not via `PLAN_ENFORCE`/deploy config.**
`envPositive` in `env.go` deliberately refuses `<= 0` — a documented safety choice ("a
typo resolving to zero... would refuse the feature to every free account and look exactly
like a deliberate decision"). Turning the intended zero into an env override would mean
weakening a guard whose own comment argues for keeping it, just to avoid a code change.
The correct lever for "sometime later, reopen a free allowance" is the *existing*
`PLAN_FREE_DAILY_<FEATURE>` override, which already accepts a positive number with no
code change — nothing new is needed for that half of the ask. This also matches
`auto-apply`'s existing precedent exactly (hardcoded `free: tierAllowance{}`,
`enforce: true`), rather than inventing a second way to reach the same state.

**Decouple "does the mechanism work" tests from "what does the product default to."**
Several tests in `decide_test.go`, `tier_test.go`, `session_test.go` and
`store_integration_test.go` use `FeatureFit` through `DefaultConfig()` (or the package's
`enforcing()` helper) as a generic stand-in with room to consume partway into an
allowance — not because `match`'s specific number matters to what they're testing.
Two changes keep this working without inventing a parallel feature:
- `enforcing()` (in `decide_test.go`, already the single helper every enforcing-config
  test in the package uses) pins `FeatureFit`'s free daily to a fixed test value via the
  already-existing `Config.WithFreeDaily` — the exact mechanism
  `internal/api/handler/*_integration_test.go` already uses for the same purpose. This
  keeps every test built on `enforcing()` — the large majority of the affected ones —
  unchanged in what they assert, only in how the number reaches them.
- A new `notEnforcing()` helper (`DefaultConfig()` with every feature's `enforce` forced
  back off) replaces the handful of tests that specifically exercise shadow-mode
  behavior — behavior that no longer exists in `DefaultConfig()` itself once every
  feature enforces by default. These tests do not depend on `FeatureFit`'s free number
  (shadow mode allows regardless of it), so no pinning is needed there.
- Tests whose entire premise inverts (e.g. "every feature but auto-apply ships not
  enforcing", "every feature but auto-apply is free-reachable") are rewritten to assert
  the new invariant rather than kept with updated numbers, since keeping the old name and
  shape would misdescribe what is actually being guaranteed now.
- `internal/api/handler/*_integration_test.go` tests that already call
  `.WithFreeDaily(...)` explicitly need no change: they were already decoupled from the
  product default.

**`PlanView.svelte` fetches the public `/api/v1/plans` prices to decide whether to show
"Upgrade to Ultra".** The endpoint (`plansHandlers.GetPlans`) already reports exactly what
is for sale, reading the same `billing.Service.PublicPrices` that drives `/pricing`'s own
column visibility — reusing it keeps one source of truth instead of adding a second signal
(e.g. a boolean on `PlanState`) for the same fact. The fetch is scoped to when it is
actually needed (`plan.plan === 'pro'`), following the file's existing convention of
scoping the billing-overview fetch to `paid` accounts only, rather than fetching it
unconditionally for every visit to the page.

## Risks / Trade-offs

- **[Risk]** Free users of `tailor`/`match`/`assistant`/`dictation`/`cover-letter` lose
  access outright the moment this ships, with no grace period or announcement built into
  this change. → **Mitigation**: this is the explicit, deliberate product decision this
  change implements (see proposal.md - Why); any grace period or in-product messaging
  beyond the existing 402 body and `plan_view.go` refusal copy is a separate product
  decision, not silently added here.
- **[Risk]** The Pro/Ultra fair-use guard numbers (e.g. tailor pro=40/day, ultra=120/day)
  were derived from the same pre-paywall usage data as the free numbers and have not been
  separately re-verified now that enforcement is live for paying accounts too. →
  **Mitigation**: these guards sit roughly twenty times above measured human behaviour by
  design (`internal/ai/plan/AGENTS.md`) and exist to stop automation draining the gateway,
  not to bound an ordinary subscriber — the same reasoning `auto-apply`'s admittedly
  unmeasured Ultra guard already ships under. Not re-deriving them is consistent with
  existing precedent, not a new risk this change introduces.
- **[Risk]** Root `AGENTS.md`'s "Plan limits" convention currently states, unqualified,
  that "a plan differs in how much ... never in whether the feature exists" — this change
  makes that false for five more features. → **Mitigation**: the convention line is
  updated in this change (see tasks.md) to describe the free tier's zero allowance as the
  documented exception it now is, matching how the same section already documents
  `auto-apply` as one.

## Migration Plan

No data migration. This is a config-default and doc change plus one frontend fetch;
`internal/ai/plan` reads no new environment variable and no schema changes. Rollback is a
revert of the `DefaultConfig()` values (or, if only a specific feature needs to reopen
immediately, setting its `PLAN_FREE_DAILY_<FEATURE>` on the host without a deploy at all).
