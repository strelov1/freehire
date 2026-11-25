## 1. Surface the specific refusal reason

- [x] 1.1 Add a failing test in `web/src/routes/my/profile/experience/page.spec.ts` (new file) asserting: (a) when `api.resetBaseCvFromResume()` rejects with an `ApiError`-shaped error carrying a specific message (e.g. the list-cap refusal text), the page shows that exact message after the candidate confirms the refresh prompt; (b) when it rejects with something carrying no useful message, the page falls back to the existing generic string.
- [x] 1.2 In `web/src/routes/my/profile/experience/+page.svelte`, import `errorMessage` from `$lib/utils` and change `offerRefreshAfterBankEdit`'s catch block from a hardcoded string to `errorMessage(e, 'Could not update your base CV. Try Reset from résumé in a tailoring workspace.')`.
- [x] 1.3 Run the new test and confirm both cases pass.

## 2. Verification

- [x] 2.1 Run `pnpm exec svelte-check` and the full `pnpm exec vitest run` — confirm 0 errors and no regressions.
- [x] 2.2 Re-read the diff: one file, one catch block, no unrelated changes.
