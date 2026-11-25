## 1. Shared layout

- [x] 1.1 Create `web/src/routes/my/profile/+layout.svelte`: owns `status` (drives the loading/error gate) and the `profileStore`/`resumeStore.ensureLoaded()` kickoff. `profileStore`/`resumeStore` themselves stay the existing app-wide singletons — leaf pages import them directly, no plumbing through the layout.
- [x] 1.2 In the layout, render the "no profile yet" setup branch (`ProfileForm` with `profile={null}` + `AccountPreferences`) when `profileStore.profile === null`, for any child route — do not render `{@render children()}` in that case.
- [x] 1.3 In the layout, render the 8-item tab strip as `<a href>` elements (Profile, Contacts, Location, Skills, Experience, Education, Screening answers, Settings), `aria-selected` from `page.url.pathname === <section href>`, keeping the current underline+icon classes. Wire `use:tablist={path}` from `$lib/actions/tablist` for keyboard nav; drop the old `handleTabKeydown`.
- [x] 1.4 Create `web/src/routes/my/profile/actions.ts` with the plain (non-reactive) shared mutation callbacks that only touch the existing singleton stores: `handleSaved`/`syncProfileAlert`, `handleCvUploaded`, `handleCvDeleted`. Render `{@render children()}` inside the existing `role="tabpanel"` wrapper in the layout when a profile exists.

## 2. Leaf routes

- [x] 2.1 Narrow `web/src/routes/my/profile/+page.svelte` to the Profile section only (`ProfileForm` + `CvSummaryCard` + `RoleCard`), importing `profileStore`/`resumeStore` and the `actions.ts` callbacks directly.
- [x] 2.2 Add `web/src/routes/my/profile/contacts/+page.svelte` (`CandidateContactsEditor`); delete the existing `contacts/+page.ts` redirect stub.
- [x] 2.3 Add `web/src/routes/my/profile/location/+page.svelte` (`LocationCard`, keyed on `profile.updated_at` as today).
- [x] 2.4 Add `web/src/routes/my/profile/skills/+page.svelte` (`SkillsCard`); delete the existing `skills/+page.ts` redirect stub.
- [x] 2.5 Add `web/src/routes/my/profile/experience/+page.svelte` (`ExperienceBankView`), with its own local `actionError` state and `offerRefreshAfterBankEdit` callback (moved from the old shared page — scoped here now since Experience was the only view that ever showed it, and a separate route unmounts it on navigation instead of needing a manual reset effect); delete the existing `experience/+page.ts` redirect stub.
- [x] 2.6 Add `web/src/routes/my/profile/education/+page.svelte` (`EducationCard`).
- [x] 2.7 Add `web/src/routes/my/profile/screening/+page.svelte` (`ScreeningAnswersForm`), with its own local `screeningAnswers` state + load (moved from the old shared page — scoped here now since only Screening ever read it); delete the existing `screening/+page.ts` redirect stub.
- [x] 2.8 Add `web/src/routes/my/profile/settings/+page.svelte` (`AccountPreferences`).

## 3. Compatibility

- [x] 3.1 Add `web/src/routes/my/profile/+page.ts` with a `load({ url })` that 308-redirects `?tab=<id>` (for the 7 known non-default ids) to `/my/profile/<id>`.
- [x] 3.2 (found in code review) Rename `web/src/routes/my/profile/cv-readiness/+page.svelte` to `+page@my.svelte` — the new `+layout.svelte` would otherwise silently wrap this pre-existing, deliberately-unlisted route too (SvelteKit nests every page inside every ancestor layout by default). Verified live: the tab strip no longer appears on `/my/profile/cv-readiness`.

## 4. Verification

- [x] 4.1 `pnpm --dir web check` (svelte-check) passes with no orphaned imports/types.
- [x] 4.2 Manual pass in the running dev stack: each of the 8 URLs loads on a fresh navigation (not just a client-side tab click) and shows the right section with its tab marked selected. Verified with a scripted Playwright pass against the live dev stack — all 8 routes select the correct tab on a cold navigation, and the Experience route renders real seeded content.
- [x] 4.3 Manual pass: with no profile, all 8 URLs show the setup form (no tab strip). The live Playwright pass for this specific case hit an unrelated pre-existing redirect (the root layout's onboarding gate, `$lib/onboardingGate.svelte.ts`, fires for any fresh session with no résumé regardless of this change) before ever reaching `/my/profile`. Verified instead by inspection: the new `+layout.svelte`'s `profile === null` branch is a byte-for-byte copy of the old page's, and it renders purely from `status`/`profileStore.profile` — it never reads the current path, so it is unaffected by which of the 8 routes triggered it.
- [x] 4.4 Manual pass: `/my/profile?tab=experience` (and the other 3 previously-consolidated ids) redirects to the matching `/my/profile/<id>`. Verified live via Playwright.
- [x] 4.5 Manual pass: arrow-key/Home/End navigation moves focus across the tab strip per the WAI-ARIA tabs pattern. Verified live via Playwright: ArrowRight/End/Home move focus correctly and activation stays manual (arrows alone do not navigate).
