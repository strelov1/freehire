## 1. `CompanyPicker`: bindable free text, optional `onSelect`

- [x] 1.1 Add a failing test in `web/src/lib/components/CompanyPicker.spec.ts` (new file) asserting: (a) typing into the input updates a bound `value` prop on every keystroke, even before any suggestion is picked; (b) picking a suggestion sets both the bound `value` (to the canonical name) and fires `onSelect` with `{slug, name}`; (c) the component renders and behaves identically when `value` is left unbound and `onSelect` is the only prop passed (mirrors today's `ReferralsView`/`MentorProfileEditor` usage).
- [x] 1.2 In `CompanyPicker.svelte`, turn the internal `query` state into a `value = $bindable('')` prop and make `onSelect` optional (`onSelect?: (...) => void`), updating `pick()`/`onInput()` to read/write `value` instead of `query`.
- [x] 1.3 Confirm `ReferralsView.svelte` and `MentorProfileEditor.svelte` still type-check and their existing tests still pass unchanged (no edits expected there).

## 2. Add-job form: company autocomplete

- [x] 2.1 Add a failing test in `web/src/lib/components/ExperienceBankView.spec.ts` (new file, or extend an existing one if present) asserting the "New experience" form's Company field shows catalogue suggestions from a mocked `api.listCompanies`, that picking one fills the field with the canonical name, and that saving with a typed name that matches no suggestion still calls `api.createExperienceEmployment` with that free text as `company`.
- [x] 2.2 In `ExperienceBankView.svelte`, replace the plain `Input` at the Company `FormField` (~lines 530-534) with `<CompanyPicker bind:value={jobCompany} />`.
- [x] 2.3 Run the new test and confirm `createJob`'s payload shape is unchanged (still `company: jobCompany.trim()`).

## 3. Edit form: company autocomplete

- [x] 3.1 Add a failing test (new or extended `web/src/lib/components/ExperienceEmploymentCard.spec.ts`) asserting: editing a `kind: 'job'` employment renders `CompanyPicker` for the Company field with catalogue suggestions available, while editing a `kind: 'project'` employment still renders a plain text "Project name" input with no suggestions.
- [x] 3.2 In `ExperienceEmploymentCard.svelte`, branch the Company/Project-name `FormField` (~lines 148-152) by `employment.kind`: `CompanyPicker bind:value={empName}` for `'job'`, unchanged `Input` for `'project'`.
- [x] 3.3 Run the new test and confirm `saveEdit`'s payload shape is unchanged for both kinds.

## 4. Add-job form: "I currently work here"

- [x] 4.1 Extend `ExperienceBankView.spec.ts` with a failing test: checking "I currently work here" hides the End-date input and makes `createJob` call `api.createExperienceEmployment` with `current: true` and `end: undefined`; leaving it unchecked keeps today's behavior (`current: false`/absent, whatever the End input holds).
- [x] 4.2 Add `jobCurrent = $state(false)` next to `jobStart`/`jobEnd`; add a checkbox ("I currently work here") beside the End `PeriodDateInput`, conditionally rendering that input with `{#if !jobCurrent}` and clearing `jobEnd` when checked.
- [x] 4.3 Pass `current: jobCurrent` in `createJob`'s request body, and reset `jobCurrent = false` alongside the other field resets after a successful save.

## 5. Edit form: "I currently work here"

- [x] 5.1 Extend `ExperienceEmploymentCard.spec.ts` with a failing test: for a `kind: 'job'` employment, `startEdit` pre-checks the control when `employment.current` is true; checking/unchecking it toggles the End-date input the same way as the add-job form; for `kind: 'project'`, no such control is rendered at all.
- [x] 5.2 Add `empCurrent = $state(false)`, initialize it from `employment.current ?? false` in `startEdit()`, and render the checkbox + conditional End `PeriodDateInput` only when `employment.kind === 'job'`.
- [x] 5.3 Pass `current: empCurrent` in `saveEdit`'s body for job-kind employments (project-kind body construction is unchanged).

## 6. Verification

- [x] 6.1 Run the full frontend check suite on the changed area: `pnpm --filter web check` (svelte-check) and the relevant vitest projects (`components`, unit) covering the new/changed spec files.
- [x] 6.2 Start the dev server and manually exercise both forms in a browser: add a job with a catalogue company via autocomplete, add one with a free-text company not in the catalogue, mark one current and confirm "Present" renders on save, edit an existing job entry to toggle current on and off.
- [x] 6.3 Re-read the diff for orphaned code (e.g. `query`-named leftovers in `CompanyPicker.svelte`) and confirm `ReferralsView.svelte`/`MentorProfileEditor.svelte` were not touched.
