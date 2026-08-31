## 1. Schema

- [x] 1.1 Migration: add `users.pro_until timestamptz NULL` (additive; no default, no backfill)
- [x] 1.2 Migration: create `usage_ledger` (user_id, feature, day date, ref, delta, kind, created_at) with a partial unique index on `(user_id, feature, ref)` for consumption entries, mirroring the debit-idempotency index `credit_ledger` uses
- [x] 1.3 Migration: create `usage_daily` (user_id, feature, day, used) — the materialised counter read on the hot path — with `(user_id, feature, day)` as the key
- [x] 1.4 Write the sqlc queries for the ledger and counter (ensure-row, select-for-update, insert-consumption, insert-release, delete-consumption, read-day, read-plan), then `make sqlc` and commit the regenerated output

## 2. The plan/allowance package

- [x] 2.1 Create `internal/ai/plan` and register it in `internal/platform/arch/layering/blocks.go`; confirm `golangci-lint run` and the layering test both pass with the new package in the table
- [x] 2.2 Plan configuration: one structure naming each plan, each feature, its daily allowance, its fair-use guard, and its enforcement switch — with defaults from the spec (2 tailor sessions, 3 fit analyses, 10 assistant messages, 10 dictations) and enforcement OFF for every feature
- [x] 2.3 `Resolve(userID) → Plan`: reads `users.pro_until` only, future = pro, NULL/past = free; assert by test that it makes no network call
- [x] 2.4 `Consume(userID, feature, ref) → Decision`: atomic reserve with `SELECT FOR UPDATE`, lazy day rollover, idempotent by `(user, feature, ref)`, returning allowed/refused plus used/allowed/reset
- [x] 2.5 `Release(userID, feature, ref)`: returns a consumption, idempotent, and a no-op when nothing was consumed — port the semantics and the month-boundary ordering trap from `credits.Release`
- [x] 2.6 `Usage(userID) → per-feature used/allowed/reset`, reporting a pro-plan caller as unlimited rather than as a number
- [x] 2.7 Shadow mode: with enforcement off, `Consume` records and reports but never refuses; with it on, the same path refuses. One code path, one switch
- [x] 2.8 Fair-use guard for the pro plan: refuse past the guard, and emit an operator-visible signal when it fires (never silent — see the design's open question)
- [x] 2.9 Concurrency test: two simultaneous consumptions with one allowance left — exactly one succeeds, recorded consumption never exceeds the allowance

## 3. Tailoring: two bounds

- [x] 3.1 Charge one tailoring-session allowance when the bootstrap creates a NEW session; returning to an existing session charges nothing (ref = the session id)
- [x] 3.2 Turn ceiling: derive the ceiling in force from the count of ledger entries for the session (`<session_id>#<n>` refs) and the turns used from `assistant_messages`; refuse the turn past the ceiling with 402 naming the session
- [x] 3.3 "Continue" consumes one further tailoring allowance and writes the `#n+1` ref; assert a double-clicked continue consumes exactly one
- [x] 3.4 Assert a tailoring turn does NOT draw on the assistant-message allowance, and that an exhausted assistant allowance does not block a tailoring turn
- [x] 3.5 Release the session allowance when a bootstrap fails before a usable session exists

## 4. The other metered features

- [x] 4.1 Fit analysis: replace the credit debit in `internal/candidate/fitanalysis` with the allowance; keep the existing reserve-before / release-on-nothing shape from d5df7abd; keep recompute free
- [x] 4.2 Assistant chat and profile presets: consume one allowance per turn before the first model call, release on terminal failure, and charge a resumed turn exactly once — exercise this against the `assistant-turn-survives-disconnect` behaviour
- [x] 4.3 Dictation: consume one allowance per accepted transcription before the audio goes upstream; keep the per-caller rate limit and keep 402 distinguishable from 429
- [x] 4.4 Remove the contribution reward from `CreateContribution` and the Telegram webhook's shared `rewardContribution` helper, leaving the contribution recorded and attributed
- [x] 4.5 Delete the credit debits and `credits.Store` call sites once nothing calls them; leave the `credit_ledger` tables in place

## 5. HTTP surface

- [x] 5.1 Replace `GET /api/v1/me/credits` and `/me/credits/history` with the plan/usage surface; the word "credits" appears in no response field
- [x] 5.2 Re-point the 402 body (`creditsError` / `renderCreditsRefusal`) at the new refusal: feature, reset instant, upgrade destination
- [x] 5.3 Assert the streaming fit endpoint and the assistant SSE issue 402 as a real status BEFORE the stream opens, not as a frame inside a 200
- [x] 5.4 `GET /api/v1/jobs/:slug/fit` reports today's allowance (used/allowed/reset, or unlimited for pro) instead of the 30-day `quota` object
- [x] 5.5 Confirm a `cv`-scoped API key is still refused with 403 on every allowance-consuming endpoint
- [x] 5.6 `/me/usage` reports over the UTC day, matching the period the allowances reset on
- [x] 5.7 Update `web/static/openapi.yaml` for every changed and removed endpoint, and check it still validates

## 6. Account deletion

- [x] 6.1 Erase `usage_ledger`, `usage_daily` and clear `pro_until` on account deletion, alongside the existing `credit_ledger` / `credit_balances` erasure
- [x] 6.2 Test: no row keyed to a deleted user survives in either the old or the new tables
- [x] 6.3 Deletion surface copy: list the plan among what is erased, and state that deletion does not cancel a subscription held by the payment provider

## 7. SPA

- [x] 7.1 Re-cast the credits page and `CreditsView.svelte` as today's usage per feature — used, allowed, reset — with no balance and no "credits"
- [x] 7.2 Handle 402 on every metered surface: name the exhausted feature, the reset time, and the upgrade path
- [x] 7.3 Tailor preflight prompt states what starting a session costs and how many remain; with none left it says so instead of offering a confirm that would be refused
- [x] 7.4 Show the per-session turn ceiling as it is approached, and offer "continue" when it is reached
- [x] 7.5 Remove every remaining "credits" string from the SPA copy

## 8. Configuration and ops

- [x] 8.1 Remove `CREDITS_MONTHLY_GRANT`, `CREDITS_COST_MATCH`, `CREDITS_COST_TAILOR`, `CREDITS_CONTRIBUTION_REWARD` from config and from the deployed env
- [x] 8.2 Document the plan configuration and the enforcement switches where the other worker/ops knobs are documented
- [x] 8.3 Add an `internal/ai/plan/AGENTS.md` covering the two-bound rule, the `#n` ref convention, shadow mode, and why the package lives in the `ai` block

## 9. Shadow run and rollout

- [ ] 9.1 Deploy with enforcement off; verify the ledger fills and nothing is refused
- [ ] 9.2 After a week, read the shadow ledger: per feature, how many users would have been refused and where in their day; record the numbers in the change before adjusting any ceiling
- [ ] 9.3 Adjust the allowances in configuration if the shadow run contradicts them
- [ ] 9.4 Enable enforcement feature by feature — fit analysis first, tailoring last — shipping each feature's SPA copy in the same release as its enforcement
