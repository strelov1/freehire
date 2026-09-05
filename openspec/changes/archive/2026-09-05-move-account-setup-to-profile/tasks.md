## 1. Step tab targeting (`accountCompleteness.ts`)

- [x] 1.1 In `web/src/lib/accountCompleteness.test.ts`, add a failing assertion that `accountSteps()` gives the `role`, `skills`, and `location` steps a `tab` of `'profile'`, `'skills'`, and `'location'` respectively, and that `cv`/`alerts` carry no `tab`.
- [x] 1.2 In `web/src/lib/accountCompleteness.ts`, add an optional `tab?: string` field to `CompletenessStep` and set it on those three steps so the test passes.

## 2. Checklist link targeting (`AccountSetupCard.svelte`)

- [x] 2.1 In `web/src/lib/components/AccountSetupCard.svelte`, append `?tab=<step.tab>` to the outstanding-step link's `href` when the step names one, composing it the same way `ReferralsLandingView.svelte` does (`resolve(step.href)` plus a manually appended query suffix — see design.md Decisions).

## 3. Move the checklist from tracking to profile

- [x] 3.1 In `web/src/routes/my/tracking/+layout.svelte`, remove the `AccountSetupCard` import and its render; leave the "Tracking" heading and the Board/List/Pipeline/Calendar tabs unchanged.
- [x] 3.2 In `web/src/routes/my/profile/+page.svelte`, import `AccountSetupCard` and render it above the tab strip, only in the branch where `profile` is already loaded (not the pre-profile setup branch).

## 4. Keep the profile tab in sync with the URL

- [x] 4.1 In `web/src/routes/my/profile/+page.svelte`, add an `$effect` that reads `page.url.searchParams.get('tab')` and, when it names a valid tab different from the current `view`, updates `view` to it — so a checklist link followed while already on `/my/profile` switches the visible section instead of only changing the query string.

## 5. Verify

- [x] 5.1 Run `pnpm --dir web test` and confirm `accountCompleteness.test.ts` passes.
- [x] 5.2 Run `pnpm --dir web check` and `pnpm --dir web lint` over the touched files.
- [ ] 5.3 (Deferred by user request — skipped in favor of the automated checks in 5.1/5.2; not verified live.) In the dev server, manually verify: the profile page shows the checklist above the tabs when the profile exists and steps are outstanding; each outstanding step opens its own tab both from a cold navigation and while already on the profile page viewing a different tab; the checklist disappears once every step is done; the pre-profile empty state shows no checklist; `/my/tracking` no longer shows the checklist.
