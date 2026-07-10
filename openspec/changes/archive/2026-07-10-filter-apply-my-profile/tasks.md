## 1. Staged store: reset-and-seed from a profile

- [x] 1.1 Add a unit test (vitest) for the reset-and-seed logic. Implemented as `profileFilters.test.ts` over the pure `filtersFromProfile` (the node vitest config has no Svelte plugin, so `.svelte.ts` runes can't be unit-tested; the pure function is the testable seam): seeds each specialization as a `category` value and each skill as a `skills` value, nothing else; trims/dedupes; empty inputs yield an empty filter set.
- [x] 1.2 Implement the reset-and-seed: pure `filtersFromProfile` in `facetModel.ts`, with `StagedFilters.applyProfile` (`stagedFilters.svelte.ts`) a thin wrapper. Tests green.

## 2. Shell header slot

- [x] 2.1 Add an optional `headerAction` snippet prop to `FilterModalShell.svelte` and render it in the header row (next to title/close). Absent slot renders nothing; `CompanyFilterModal` unaffected.

## 3. Wrapper wiring and gating

- [x] 3.1 In `FilterModal.svelte`, warm `profileStore.ensureLoaded()` when the modal opens for a signed-in user in full-job scope (extended the existing Telegram-flag warm-up effect).
- [x] 3.2 Pass a `headerAction` snippet to the shell that, in full-job scope only: signed-in with a saved profile → renders an **Apply my profile** button calling `staged.applyProfile(profile.specializations, profile.skills)`; signed-in without a profile → renders a link to `/my/profile`; signed-out → renders nothing. Gated on `profileStore.loaded` (made reactive) so a user with a profile never flashes the create link while it loads.
- [x] 3.3 Confirm the reuse paths (profile-comparison modal with `railKeys`, `CompanyFilterModal`) show no profile action — verified: `railKeys` clears `hasSavedTab`; `CompanyFilterModal` omits the slot.

## 4. Verification

- [x] 4.1 Run `svelte-check` and the vitest suite; both clean (116 tests pass, 0 check errors).
- [ ] 4.2 Visual verify in the running web app: open the jobs filter modal signed-in with a profile → **Apply my profile** resets and seeds `category`/`skills`, preview count updates, **Show results** commits to the URL; signed-in without a profile shows the create-profile link; signed-out shows neither. (Requires the running SPA + API + a signed-in user with a profile.)
