## 1. Core config change

- [x] 1.1 In `internal/ai/plan/plan.go`, change `DefaultConfig()` for `FeatureTailor`,
      `FeatureFit`, `FeatureAssistant`, `FeatureDictation`, `FeatureCoverLetter`: `free`
      becomes `tierAllowance{}` (zero) and `enforce` becomes `true`, mirroring
      `FeatureAutoApply`'s existing shape. Leave `pro`/`ultra` numbers untouched.
- [x] 1.2 Update the doc comment above `DefaultConfig()` (currently explains why
      enforcement ships off for every feature) and the comment above `FeatureCoverLetter`
      (currently says it "ships with enforcement OFF like every other AI feature") to
      describe the new default and why (see design.md - Decisions).

## 2. `internal/ai/plan`'s own test suite

- [x] 2.1 In `decide_test.go`: add a `notEnforcing()` helper (`DefaultConfig()` with every
      feature's `enforce` forced back off via the existing unexported `Config.with`), and
      switch `TestDecideShadowRecordsButNeverRefuses` and
      `TestFairUseGuardHoldsEvenInShadow` from bare `DefaultConfig()` to it.
- [x] 2.2 In `decide_test.go`: pin `enforcing()`'s `FeatureFit` free daily to a fixed test
      value via the existing `Config.WithFreeDaily` (e.g. 3, matching the numbers already
      asserted by `TestDecideWithinAllowance`/`TestDecideRefusesAtTheAllowance`), so those
      and the other `enforcing()`-based tests keep exercising real within/at-allowance
      behaviour instead of a vacuous zero-limit.
- [x] 2.3 In `plan_test.go`: update `TestDefaultConfigMatchesTheSpec`'s free-daily map to 0
      for `FeatureTailor`/`FeatureFit`/`FeatureAssistant`/`FeatureDictation` and add
      `FeatureCoverLetter: 0`. Update `TestAllowanceForTier`'s `FeatureFit` assertion from
      `Limit != 3` to `Limit != 0`.
- [x] 2.4 In `plan_test.go`: replace `TestEnforcementStartsOff` and
      `TestEveryPlanOffersEveryFeature` (both invert under the new default) with a single
      test asserting the new invariant for every feature in `AllFeatures()`: free daily is
      0, `Enforced()` is true, and the pro allowance is > 0 (a subscription unlocks
      something).
- [x] 2.5 In `tier_test.go`: update `TestAllowanceAnswersForEveryTier`'s free-tailor
      assertion from `Limit != 2` to `Limit != 0`. Rewrite
      `TestAutoApplyEnforcesOnArrival` (currently asserts the other five features do NOT
      enforce) into a test asserting every feature in `AllFeatures()` enforces.
- [x] 2.6 In `env_test.go`: simplify `TestConfigFromEnvDefaultsToTheShippedConfig` to
      assert every feature is enforced with nothing set (drop the auto-apply special
      case, no longer needed). Rewrite `TestEnforcementIsNamedPerFeature`,
      `TestEnforceAllIsSpelledOut` and `TestAnUnknownFeatureNameIsIgnoredNotGuessed` to
      call the unexported `enforcedFeatures(list string)` parser directly instead of
      going through `ConfigFromEnv()` + `Enforced()` — the per-name behaviour they test
      can no longer be observed through defaults that are already all-true.
- [x] 2.7 In `session_test.go`: switch `TestShadowModeDoesNotStopATurn` from bare
      `DefaultConfig()` to `notEnforcing()` (it relies on `Enforced(FeatureTailor)` being
      false, which `decideTurn` reads directly).
- [x] 2.8 In `store_integration_test.go`: switch `TestShadowModeRecordsWithoutRefusing`
      (line ~417, currently bare `DefaultConfig()`) to `notEnforcing()`. Grep the file for
      any other bare `DefaultConfig()` or unpinned `enforcing()` use and fix the same way.

## 3. `internal/api/handler` integration test fixtures

Known-affected fixtures (found by auditing every `plan.DefaultConfig()` call site in
`internal/api/handler/*_integration_test.go`):

- [x] 3.1 `auto_apply_tailor_integration_test.go`: `newAutoApplyTailorApp`'s bare
      `plan.DefaultConfig()` (line ~47) feeds ~9 test functions; the ones that actually
      call `POST /tailor` expecting success
      (`TestPostAutoApplyTailor_RunsAndRecordsTheTailoredCV`,
      `TestPostAutoApplyTailor_ASpentBudgetStillRecordsTheTailoredCV`,
      `TestPostAutoApplyTailor_AFailedRunIsRecordedAndNotified`) will 402 once tailor's
      free allowance is 0. Change the fixture to
      `plan.DefaultConfig().WithFreeDaily(plan.FeatureTailor, 1)`.
- [x] 3.2 `cv_tailor_integration_test.go`: `newTailorAPI`'s
      `plan.DefaultConfig().Enforcing()` (line ~48) is the shared fixture for the whole
      file. Pin `.WithFreeDaily(plan.FeatureTailor, N)`, picking N at least as large as
      the most sessions any single test in the file starts (check for a multi-session
      test before choosing N — do not guess a value that under-covers one).
- [x] 3.3 `assistant_extend_integration_test.go`: `TestExtendingASessionBuysMoreTurns`
      (and any other caller of the `startedTailorSession` helper that passes an unpinned
      `plan.DefaultConfig().Enforcing()`) needs `.WithFreeDaily(plan.FeatureTailor, N)`
      added at the call site, or `StartSession` refuses before the test's real assertions
      run.
- [x] 3.4 `cv_cover_letter_integration_test.go`:
      `TestCoverLetter_UnproducedDraftLeavesTheStoreAndTheAllowanceAlone` (line ~294) uses
      bare `plan.DefaultConfig()` expecting to reach the draft step (503) before its
      allowance-release assertion; with cover-letter's free allowance at 0 it now 402s
      before reaching that code path. Change to
      `.WithFreeDaily(plan.FeatureCoverLetter, 1)`.
- [x] 3.5 Where a test seeds a "spent allowance" via a dynamic
      `plan.DefaultConfig().FreeDaily(FeatureTailor)` read (`cv_tailor_integration_test.go`
      line ~438, `auto_apply_tailor_integration_test.go` line ~252) — after 3.1/3.2 pin a
      real free-daily value for the fixture those tests share, switch the seed to read
      from that same pinned config rather than the raw `plan.DefaultConfig()`, so the test
      still proves an allowance was actually spent down to it rather than starting at an
      already-zero limit.
- [x] 3.6 Do one mechanical pass over every remaining
      `internal/api/handler/*_integration_test.go` file that constructs a `plan.Config`
      (`grep -n "plan\.DefaultConfig()" internal/api/handler/*_integration_test.go`,
      cross-referencing anything not already chained with `.WithFreeDaily`): for each hit,
      trace whether the config path it feeds reaches a real `Consume`/`StartSession`/
      `Standing` call in the test(s) that use it, or is incidental plumbing (e.g. a
      handler that only reads `/me/plan` or only reaches an ownership/404 check before the
      plan gate). Confirmed-incidental so far:
      `auto_apply_review_publish_integration_test.go` (only hits `/review`, tailoring
      already seeded), `billing_store_integration_test.go`
      (`TestPlanNamesWhereProCameFrom`, read-only). Still needs a per-function check:
      `cv_evidence_gate_integration_test.go`, `me_analyses_integration_test.go`,
      `match_analysis_stream_integration_test.go`, `match_analysis_integration_test.go`.
      Fix whatever this pass finds the same way (pin `.WithFreeDaily` for the feature and
      count the test actually needs).

## 4. Frontend: honest Ultra upgrade CTA

- [x] 4.1 In `web/src/lib/components/PlanView.svelte`, fetch `api.plans()` (the existing
      public `/api/v1/plans` client call) scoped to when `plan.plan === 'pro'` (matching
      the file's existing convention of scoping the billing-overview fetch to paid
      accounts), and derive whether any `tier === 'ultra'` price is present.
- [x] 4.2 Change the CTA condition so "Upgrade to Ultra" only renders for a Pro subscriber
      when that derived flag is true; a Free subscriber's generic "Upgrade" CTA and an
      Ultra subscriber's lack of any CTA are both unaffected.
- [x] 4.3 Check for existing component/route tests covering `PlanView.svelte`'s upgrade
      CTA and update or add coverage for both states (Ultra offered / not offered).

## 5. Docs

- [x] 5.1 Update `internal/ai/plan/AGENTS.md`'s bullets that name `auto-apply` as the one
      feature enforcing on arrival / the one exception to shadow-mode-by-default, to
      reflect that the other five features now ship the same way, and why (this is a
      product decision made without a shadow run to read, unlike the earlier per-feature
      rollout `PLAN_ENFORCE` was built for).
- [x] 5.2 Update the root `AGENTS.md`'s "Plan limits" convention line — "a plan differs in
      how MUCH ... never in whether the feature exists" needs the same qualification it
      already gives `auto-apply`, extended to the other five features.

## 6. Verification

- [x] 6.1 `gofmt -l .` clean on every touched Go file; `go vet ./...`; `go test ./...`.
- [x] 6.2 `go vet -tags=integration ./...` (compiles every integration-tagged file,
      including the handler fixtures touched in section 3).
- [x] 6.3 Behaviour changed, so run the full tagged suite rather than relying on vet
      alone: `go test -tags=integration ./internal/ai/plan/...` and
      `go test -tags=integration ./internal/api/handler/...` (needs Docker/testcontainers)
      — fix any failure surfaced beyond what section 3 already anticipated.
- [x] 6.4 Frontend: run the project's lint/typecheck for the touched Svelte file and
      manually exercise `PlanView.svelte` as both a Pro subscriber (Ultra offered and
      Ultra not offered) and a Free subscriber, per `run`/svelte-core-bestpractices
      conventions.
