## 1. Web typecheck

- [x] 1.1 In `web/src/routes/my/profile/+page.svelte`, assign `null` (not `{}`) when `facetCounts` is rejected
- [x] 1.2 Run `pnpm run check` in `web/` and confirm zero errors

## 2. Adoption ratchet

- [x] 2.1 Update `design-system/scripts/adoption-baseline.json` via `pnpm check:adoption -- --update` (or the script’s `--update` flag)
- [x] 2.2 Run `pnpm check:adoption` in `design-system/` and confirm exit 0

## 3. CV heal / reset contracts

- [x] 3.1 Give `healRecordHeader` a keep-first fill (empty fields only); leave `mergeSeedHeader` seed-first for `applySeedContent`
- [x] 3.2 Restore the stale-structure gate so reset-from-résumé does not copy summary/skills/projects from a superseded blob
- [x] 3.3 Run `go test -tags=integration` for `TestGetCV_PartialHeaderKeepsNameFillsEmail`, `TestResetCVFromResume_ProvisionalContactsPlusBankSucceeds`, and the related `cv_header_heal` / `cv_seed` / `cv_reset` tests

## 4. Extension typecheck

- [x] 4.1 Narrow `possibly undefined` in `extension/lib/form.ts`, `combobox.ts`, and `freehire.ts`
- [x] 4.2 Narrow the same class of errors in `extension/lib/form.test.ts`, `assistant/api.test.ts`, and `assistant/client.test.ts`
- [x] 4.3 Run `npm run check` in `extension/` and confirm zero errors

## 5. Ship locally (no push)

- [x] 5.1 When the user asks, commit on `feat/profile-contacts-experience-coverage` (do not push unless they explicitly request it)
