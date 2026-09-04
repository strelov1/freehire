## 1. Shared layout

- [ ] 1.1 Create `web/src/routes/my/profile/+layout.svelte`: move `profileStore`/`resumeStore` loading, `screeningAnswers` state + `loadScreeningAnswers`, `status`, `actionError`, `handleSaved`, `syncProfileAlert`, `handleCvUploaded`, `handleCvDeleted`, `offerRefreshAfterBankEdit` out of `+page.svelte` into it.
- [ ] 1.2 In the layout, render the "no profile yet" setup branch (`ProfileForm` with `profile={null}` + `AccountPreferences`) when `profileStore.profile === null`, for any child route — do not render `{@render children()}` in that case.
- [ ] 1.3 In the layout, render the 8-item tab strip as `<a href>` elements (Profile, Contacts, Location, Skills, Experience, Education, Screening answers, Settings), `aria-selected` from `page.url.pathname === <section href>`, keeping the current underline+icon classes. Wire `use:tablist={path}` from `$lib/actions/tablist` for keyboard nav; drop the old `handleTabKeydown`.
- [ ] 1.4 Expose the moved state + callbacks to child pages via `setContext`, and render `{@render children()}` inside the existing `role="tabpanel"` wrapper when a profile exists.

## 2. Leaf routes

- [ ] 2.1 Narrow `web/src/routes/my/profile/+page.svelte` to the Profile section only (`ProfileForm` + `CvSummaryCard` + `RoleCard`), reading shared state via `getContext`.
- [ ] 2.2 Add `web/src/routes/my/profile/contacts/+page.svelte` (`CandidateContactsEditor`); delete the existing `contacts/+page.ts` redirect stub.
- [ ] 2.3 Add `web/src/routes/my/profile/location/+page.svelte` (`LocationCard`, keyed on `profile.updated_at` as today).
- [ ] 2.4 Add `web/src/routes/my/profile/skills/+page.svelte` (`SkillsCard`); delete the existing `skills/+page.ts` redirect stub.
- [ ] 2.5 Add `web/src/routes/my/profile/experience/+page.svelte` (`ExperienceBankView`, wired to `offerRefreshAfterBankEdit`); delete the existing `experience/+page.ts` redirect stub.
- [ ] 2.6 Add `web/src/routes/my/profile/education/+page.svelte` (`EducationCard`).
- [ ] 2.7 Add `web/src/routes/my/profile/screening/+page.svelte` (`ScreeningAnswersForm`); delete the existing `screening/+page.ts` redirect stub.
- [ ] 2.8 Add `web/src/routes/my/profile/settings/+page.svelte` (`AccountPreferences`).

## 3. Compatibility

- [ ] 3.1 Add `web/src/routes/my/profile/+page.ts` with a `load({ url })` that 308-redirects `?tab=<id>` (for the 7 known non-default ids) to `/my/profile/<id>`.

## 4. Verification

- [ ] 4.1 `pnpm --dir web check` (svelte-check) passes with no orphaned imports/types.
- [ ] 4.2 Manual pass in the running dev stack: each of the 8 URLs loads on a fresh navigation (not just a client-side tab click) and shows the right section with its tab marked selected.
- [ ] 4.3 Manual pass: with no profile, all 8 URLs show the setup form (no tab strip).
- [ ] 4.4 Manual pass: `/my/profile?tab=experience` (and the other 3 previously-consolidated ids) redirects to the matching `/my/profile/<id>`.
- [ ] 4.5 Manual pass: arrow-key/Home/End navigation moves focus across the tab strip per the WAI-ARIA tabs pattern.
