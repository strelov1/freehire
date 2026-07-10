## 1. Staged store: reset-and-seed from a profile

- [ ] 1.1 Add a unit test (vitest) for `StagedFilters.applyProfile(specializations, skills)`: given non-empty prior staged filters, it clears everything, then stages each specialization as a `category` value and each skill as a `skills` value (assert via `params()`); empty inputs leave the store cleared.
- [ ] 1.2 Implement `applyProfile` on `StagedFilters` (`web/src/lib/stagedFilters.svelte.ts`): `clear()` then seed `category`/`skills` through the existing facet-add path. Tests green.

## 2. Shell header slot

- [ ] 2.1 Add an optional `headerAction` snippet prop to `FilterModalShell.svelte` and render it in the header row (next to title/close). Absent slot renders nothing; `CompanyFilterModal` unaffected.

## 3. Wrapper wiring and gating

- [ ] 3.1 In `FilterModal.svelte`, warm `profileStore.ensureLoaded()` when the modal opens for a signed-in user in full-job scope (mirror the existing Telegram-flag warm-up effect).
- [ ] 3.2 Pass a `headerAction` snippet to the shell that, in full-job scope only: signed-in with a saved profile → renders an **Apply my profile** button calling `staged.applyProfile(profile.specializations, profile.skills)`; signed-in without a profile → renders a link to `/my/profile`; signed-out → renders nothing.
- [ ] 3.3 Confirm the reuse paths (profile-comparison modal with `railKeys`, `CompanyFilterModal`) show no profile action.

## 4. Verification

- [ ] 4.1 Run `svelte-check` and the vitest suite; both clean.
- [ ] 4.2 Visual verify in the running web app: open the jobs filter modal signed-in with a profile → **Apply my profile** resets and seeds `category`/`skills`, preview count updates, **Show results** commits to the URL; signed-in without a profile shows the create-profile link; signed-out shows neither.
