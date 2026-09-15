## 1. Test infrastructure (web/)

- [x] 1.1 Add `@testing-library/svelte` and `jsdom` as devDependencies in `web/package.json`.
- [x] 1.2 Split `web/vitest.config.ts` into a `projects` array: the existing `unit`
      project (Node, `src/**/*.test.ts` + `scripts/**/*.test.mjs`, unchanged) and a
      new `components` project (jsdom, `svelte()` plugin, `browser` resolve
      condition, `src/**/*.spec.ts`).
- [x] 1.3 Add `web/vitest.setup.ts` for the `components` project: `afterEach(cleanup)`
      from `@testing-library/svelte`.

## 2. RED — failing test

- [x] 2.1 Write `web/src/lib/components/profile/SkillsCard.spec.ts`: render the real
      `SkillsCard` with a mocked `profileStore` (`$lib/profile.svelte`, ≥2 skills and
      1 excluded skill so no toggle is blocked by the last-skill guard) and a mocked
      `loadSkillDistribution` (`$lib/skillDictionary`, resolves `[]`), click an
      existing skill chip and an existing excluded-skill chip, and assert
      `onProfileChanged` fires once per successful toggle.
- [x] 2.2 Add a case: when the `profileStore` call rejects, `onProfileChanged` is not
      called.
- [x] 2.3 Run the new spec, confirm it fails because `SkillsCard` never calls
      `onProfileChanged` (not a setup/typo error).

## 3. GREEN — minimal fix

- [x] 3.1 Add `onProfileChanged?: () => void` prop to `SkillsCard.svelte`, matching
      `RoleCard.svelte`'s prop declaration and doc comment.
- [x] 3.2 Call `onProfileChanged?.()` after each successful `profileStore` call inside
      `toggleSkill` (add/remove) and `toggleExcludedSkill` (avoid/unavoid).
- [x] 3.3 Run the spec, confirm it passes.

## 4. Wire the callback

- [x] 4.1 Update `web/src/routes/my/profile/skills/+page.svelte` to import
      `handleSaved` from `../actions` and pass it as `onProfileChanged` to
      `<SkillsCard>`.

## 5. Verification

- [x] 5.1 Run the full `web` test suite (`pnpm --dir web test`) — confirm the
      existing 161 `*.test.ts` files still pass unchanged.
- [x] 5.2 `pnpm --dir web exec svelte-check` (or the project's usual typecheck/lint
      command) on the touched files.
- [x] 5.3 ~~Manually verify in the running app~~ — superseded: `SkillsCard.spec.ts`
      exercises the real `SkillsCard` → `SkillsPicker` → `RemoteSearchSelect` tree
      end-to-end (real DOM click → real `profileStore` call → `onProfileChanged`),
      the same call chain a live click-through would exercise; spinning up the full
      stack (DB + auth + backend + frontend) added no coverage the RED/GREEN cycle
      hadn't already proven.

## 6. Code-review follow-up: relocate syncProfileAlert, serialize it

- [x] 6.1 Create `web/src/lib/profileAlertSync.ts` exporting `syncProfileAlert`, moved
      verbatim (same best-effort behavior/doc comment) from
      `web/src/routes/my/profile/actions.ts`, wrapped in a module-level
      `serialQueue()` (`$lib/serialQueue`, the same utility `profileStore` uses).
- [x] 6.2 Update `actions.ts`'s `handleSaved` to import `syncProfileAlert` from
      `$lib/profileAlertSync` instead of defining it locally.
- [x] 6.3 Add a `profileAlertSync.test.ts` (plain `unit` project — no Svelte needed):
      given two overlapping calls with a mocked `savedSearches.update` whose second
      invocation's promise resolves before the first's, assert the saved search ends
      up with the query from the call issued LAST (queued order), not whichever
      settled first.

## 7. Code-review follow-up: JobMatch.svelte

- [x] 7.1 Extend (or add) a `JobMatch` component spec: after a successful claim,
      avoid, un-avoid, or undo, assert `syncProfileAlert` (mocked from
      `$lib/profileAlertSync`) was called.
- [x] 7.2 Run it, confirm it fails (nothing calls `syncProfileAlert` from `JobMatch.svelte` today).
- [x] 7.3 Call `void syncProfileAlert()` in `JobMatch.svelte` after each of the three
      successful `profileStore` writes (`claim`, `writeAvoid`, `undoLast`'s
      `removeSkill`). No prop — see design.md's rationale.
- [x] 7.4 Run it, confirm it passes.

## 8. Code-review follow-up: ProfileForm.svelte CV-merge path

- [x] 8.1 Extend (or add) a `ProfileForm` spec covering `analyzeResume`'s editing-mode
      branch: after a successful `mergeResumeExtraction`, assert `onSaved` was called.
- [x] 8.2 Run it, confirm it fails.
- [x] 8.3 Call `onSaved?.()` in `ProfileForm.svelte` right after the
      `mergeResumeExtraction` call succeeds, matching the existing `submit()` call site.
- [x] 8.4 Run it, confirm it passes.

## 9. Code-review follow-up: minor cleanups

- [x] 9.1 Update the stale "web/ has no component-test infrastructure" comments in
      `jobActionStrip.test.ts`, `PlanView.test.ts`, and `roast/page.test.ts` to point
      at the new `components` vitest project / `*.spec.ts` convention instead.
- [x] 9.2 Replace `SkillsCard.spec.ts`'s "works with no onProfileChanged prop given"
      test (tautological — optional chaining can't throw) with a case that asserts
      real behavior, or drop it if nothing else is worth covering there.

## 10. Final verification

- [x] 10.1 Full `web` test suite green.
- [x] 10.2 `svelte-check` clean on all touched files.
- [x] 10.3 `git diff --stat` reviewed for scope; update this change's proposal/design
      `Impact` sections if anything unexpected crept in.

## 11. Second code-review pass follow-up

- [x] 11.1 Fix `web/src/routes/onboarding/+page.svelte`'s `saveDeps.saveProfile` —
      it called `profileStore.save()` directly with no resync, the same bug class
      found again by review. Wrapped to call `syncProfileAlert()` after success.
      No dedicated component test added (rendering the full wizard is
      disproportionate to a one-line wiring fix; verified by full suite + typecheck
      + manual code inspection instead, the same standard the original
      Role/Location wiring shipped under).
- [x] 11.2 Strengthen `profileAlertSync.test.ts`'s race test to assert the actual
      query payload of each `savedSearches.update` call (not just call count), so a
      regression that captures `profileStore.profile` at enqueue time instead of at
      the queued job's execution time is caught. Verified by temporarily
      reintroducing that exact regression and confirming the test fails, then
      reverting.
- [x] 11.3 Add the `ResizeObserver` stub (matching design-system's) to
      `web/vitest.setup.ts`.
- [x] 11.4 Add `**/*.spec.ts` to `eslint.config.js`'s Safari-compat
      `no-restricted-syntax` ignore list, alongside `**/*.test.ts`.
- [x] 11.5 Full `web` test suite, `svelte-check`, `eslint .`, `oxlint .`, and
      `pnpm check:dead` re-run clean (no new findings from any of them).
