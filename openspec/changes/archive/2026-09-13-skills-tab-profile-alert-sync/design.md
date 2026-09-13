## Context

See proposal.md - Why. `RoleCard.svelte` and `LocationCard.svelte` both take an
`onProfileChanged?: () => void` prop and call it after every successful autosave;
their parent routes wire it to `handleSaved` (`web/src/routes/my/profile/actions.ts`),
which calls `syncProfileAlert()` — a best-effort call that finds the account's
`derived_from_profile` saved search (if any) and updates its `query` from the
just-saved `profileStore.profile` via `filtersFromProfile`. `SkillsCard.svelte` is
the one autosaving profile section that skips this wiring entirely.

## Goals / Non-Goals

**Goals:**
- Skill add/remove/avoid/unavoid triggers the same resync Role/Location edits do.
- Match the existing prop shape and call site exactly — no new abstraction.

**Non-Goals:**
- Changing `syncProfileAlert`, `filtersFromProfile`, or the derived-search model.
- Deduplicating the repeated "autosave + notify parent" shape across
  Role/Location/Skills — three call sites is not enough to justify extraction, and
  the proposal explicitly keeps this a like-for-like fix.

## Decisions

- **Prop name: `onProfileChanged`, not a new `onSkillsChanged`.** RoleCard already
  named it this generically (not `onRoleChanged`) precisely because the callback's
  job — telling the profile page a save happened — has nothing to do with which
  section changed. Reusing the same name keeps all three cards' call sites
  identical and greppable as one pattern.
- **Fire after each mutator resolves, not once per card mount or batched.** Every
  other autosaving section fires its callback per successful write, and
  `syncProfileAlert` is idempotent (it just re-reads the current profile and
  writes the current query), so firing it four times as separate toggles happen
  costs nothing beyond what Role/Location already do for their own multi-toggle
  interactions (e.g. toggling several specializations in a row).
- **No change to error handling.** `toggleSkill`/`toggleExcludedSkill` already only
  reach their success path when the `profileStore` call resolves; the callback
  goes there, same as `RoleCard.toggleSpecialization`'s `await ...; onProfileChanged?.();`
  placement.

- **Test infrastructure: add `@testing-library/svelte` + jsdom to `web/`, as a new,
  separate vitest project.** `web/vitest.config.ts` currently runs every test in
  plain Node deliberately ("Nothing tested here needs Svelte compilation"), and
  neither `RoleCard`/`LocationCard` (the pattern being copied) nor `SkillsCard`
  itself have any existing test. TDD still requires a failing test for this
  behavior change, and the behavior lives entirely inside a Svelte component's
  event handlers — untestable without rendering it. Rather than bending the
  existing Node project to compile Svelte (risking the 161 tests it currently
  isolates from the SvelteKit/browser resolution condition), this adds a second
  vitest `projects` entry — mirroring `design-system/vitest.config.ts`'s own
  `components`/`scripts` split — scoped to a new `*.spec.ts` extension so it
  never overlaps with the existing `*.test.ts` glob. One devDependency pair
  (`@testing-library/svelte`, `jsdom`); `@sveltejs/vite-plugin-svelte` is already
  a `web/` devDependency.
- **Drive the real `SkillsPicker`, mock only the network-backed edges.** Per
  writing-good-tests' "mock at the right level," the test renders the real
  `SkillsCard` → `SkillsPicker` → `RemoteSearchSelect` tree and clicks a real
  rendered chip (`RemoteSearchSelect` always renders `include`/`exclude` values
  as clickable chips, independent of its debounced search) rather than mocking
  `SkillsPicker` itself. Only `profileStore` (real HTTP calls) and
  `loadSkillDistribution` (real HTTP fetch of the skill dictionary) are mocked.

- **Relocate `syncProfileAlert` to `$lib`, don't thread a prop through `JobMatch.svelte`'s
  five call sites.** `RoleCard`/`LocationCard`/`SkillsCard` all live under
  `/my/profile` already, so `onProfileChanged` wired to the route's own
  `handleSaved` is the natural shape there. `JobMatch.svelte` is rendered from
  `JobView.svelte`, `JobRow.svelte`, `SwipeDeck.svelte`, `JobDrawer.svelte`, and
  `/tailor/[slug]` — none of them otherwise touch the profile-alert feature, so
  a prop would mean five unrelated call sites each remembering to import a
  route-local callback and pass it down. `syncProfileAlert` has no
  component-local dependency (it only reads global stores), so `JobMatch.svelte`
  calling it directly removes exactly the kind of wiring site review found
  missing for `SkillsCard`.
- **Serialize `syncProfileAlert` itself, not `savedSearches.update`.** The queue
  belongs to the one thing that can race across independent callers — two
  overlapping `syncProfileAlert()` invocations, from any pair of
  Role/Location/Skills/JobMatch/CV-merge. Wrapping `savedSearches.update`
  instead would serialize a lower-level primitive `create`/`delete` don't need
  to share a queue with, for a hazard that is specific to this one repeated
  caller.

## Risks / Trade-offs

- [Firing `onProfileChanged` on every toggle could mean more `PUT` traffic for the
  derived-search sync during a rapid multi-skill edit] → Already true for
  Role/Location today; `syncProfileAlert` is a single small `PATCH`-equivalent
  update, not a full reindex, and failures are swallowed best-effort exactly as
  they are for the other two sections.
- [A second vitest project adds startup/config surface to `web/`] → Scoped to a
  non-overlapping file extension (`*.spec.ts`) so the existing 161 `*.test.ts`
  files and their Node environment are untouched; confirmed by running the full
  `web` suite after the change.
- [Widening scope mid-change to `JobMatch.svelte`/`ProfileForm.svelte` beyond the
  originally-proposed `SkillsCard`-only fix] → Both are verified, same-bug-class
  gaps (real `profileStore` writes with no alert resync), found by code review
  against the actual codebase, not speculative — shipping `SkillsCard` alone
  would leave the higher-traffic job-match surface exhibiting the identical
  staleness this change exists to fix.
