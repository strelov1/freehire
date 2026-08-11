## Why

PR #1737 is red after the rebase onto `origin/main` (`986d24a1`). Four checks fail; none of them should change product requirements — they are type, ratchet, test-contract, and extension typecheck failures that block merge. Do not push until the user asks.

## What Changes

- **web Type check:** `my/profile/+page.svelte` assigns `{}` when facet counts fail; svelte-check rejects `FacetCounts | {}` vs `FacetCounts | null`. Use `null` (same degrade path the comment already describes).
- **design-system Check adoption:** Button `51 → 53` and Input `11 → 12` are real extra consumers. Record them with `pnpm check:adoption -- --update` (ratchet is exact in both directions).
- **backend Integration tests:** restore the existing heal/reset contracts:
  - `TestResetCVFromResume_ProvisionalContactsPlusBankSucceeds` — summary came back `"stale"`; provisional seed must not copy résumé semantics.
  - `TestGetCV_PartialHeaderKeepsNameFillsEmail` — name came back `"From Blob"`; heal must fill missing email only and keep `"Keep Me"`.
- **extension build (Type check):** 47 `possibly undefined` errors in 6 files (`form.ts`, `combobox.ts`, `freehire.ts`, `form.test.ts`, `assistant/api.test.ts`, `assistant/client.test.ts`). This workflow runs on this PR because the branch touches `design-system/`. The same check is already red on `main` after #1733; it still blocks this PR’s `build` check, so fix the type errors here.
- Leave pre-existing oxlint warnings alone. No remote push as part of apply.

## Capabilities

### New Capabilities

<!-- none — CI unblock only; skip_specs: true -->

### Modified Capabilities

<!-- none — restore existing contracts, no new requirements -->

## Impact

- `web/src/routes/my/profile/+page.svelte`
- `design-system/scripts/adoption-baseline.json`
- CV header-heal / reset-from-resume path (`internal/handler/cv_header_heal.go`, `cv_reset.go` / seed) — behaviour, not the tests’ assertions
- `extension/lib/{form,combobox,freehire}.ts` and the listed tests
- Local verify: `pnpm run check` in `web/`, `pnpm check:adoption` in `design-system/`, `npm run check` in `extension/`, `go test -tags=integration` for the two named tests
- Delivery: commit on `feat/profile-contacts-experience-coverage` when asked; **do not push** until the user explicitly requests it
