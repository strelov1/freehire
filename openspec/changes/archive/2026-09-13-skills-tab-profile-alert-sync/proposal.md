## Why

`profile-alert-toggle`'s own "Profile-derived search stays in sync" requirement says
the profile-derived saved search ("Jobs matching my profile") is recomputed and
updated **every time the account's candidate profile is saved** — not just on some
sections. In the current implementation, `RoleCard.svelte` and `LocationCard.svelte`
both call an `onProfileChanged` callback after every autosave, wired to `handleSaved`
(`web/src/routes/my/profile/actions.ts`), which re-derives and updates that search.
`SkillsCard.svelte` (the Skills tab, `/my/profile/skills`) autosaves skill
add/remove/avoid/unavoid straight through `profileStore` but never calls any such
callback, so a skill-only edit silently leaves the derived search — and therefore
every notification channel subscribed to it — matching the OLD skill set.

Found live on prod while diagnosing a real account: its profile-derived alert had
been stale since 2026-08-17 despite a profile save on 2026-09-08 that changed
`specializations`, `skills`, and `excluded_skills` through the Skills tab. Nothing
told the user the alert had drifted; it looked like "notifications aren't sending"
when the underlying subscription was simply watching a filter the account no longer
has.

## What Changes

- `SkillsCard.svelte` gains an `onProfileChanged` callback prop, matching the exact
  shape `RoleCard.svelte`/`LocationCard.svelte` already use. It is called once
  after each of the four `profileStore` calls
  (`addSkill`/`removeSkill`/`avoidSkill`/`unavoidSkill`) succeeds.
  `web/src/routes/my/profile/skills/+page.svelte` wires that prop to the existing
  `handleSaved` from `./actions` (the same function Role/Location already use).
- `syncProfileAlert` moves out of the route-local `actions.ts` into
  `$lib/profileAlertSync.ts`, unchanged in behavior, so components outside
  `/my/profile/*` can call it directly instead of importing a route file (a
  route-into-lib import direction the codebase doesn't otherwise use) or having a
  callback threaded down to them from an unrelated route.
- Two more real skill-mutating surfaces found by review, same bug class as
  `SkillsCard`, get the same fix:
  - **`JobMatch.svelte`** (claim/avoid/undo a skill from a job's match block —
    reachable from `JobView.svelte`, `JobRow.svelte`, `SwipeDeck.svelte`,
    `JobDrawer.svelte`, and `/tailor/[slug]`) now calls `syncProfileAlert()`
    directly after each of its three successful `profileStore` writes. Called
    directly (not via a prop) because none of its five call sites has any other
    reason to know about the profile-derived alert, and threading a callback
    through five sites is the exact failure mode this fix closes.
  - **`ProfileForm.svelte`**'s CV-re-upload path, in editing mode, already calls
    `profileStore.mergeResumeExtraction` but never called `onSaved?.()` after it —
    unlike the form's own Save-button submit, which does. One line added.
- **`onboarding/+page.svelte`**'s per-step wizard save (`saveDeps.saveProfile`)
  also called `profileStore.save()` directly with no sync — the same bug class,
  found by a second review pass. Wrapped to call `syncProfileAlert()` after a
  successful save, same as every other site.
- `syncProfileAlert` gains a module-level `serialQueue()` (the same utility
  `profileStore` already uses) around its body, so concurrent callers (any
  combination of Role/Location/Skills/JobMatch/CV-merge/onboarding) can no longer
  race two `PUT`s and have the stale one land last. The race test asserts on the
  actual query payload each call sent, not just call counts — a regression that
  captured `profileStore.profile` at enqueue time instead of at the queued job's
  own execution time would still pass a count-only assertion.
- `web/vitest.setup.ts` gains the same `ResizeObserver` stub
  `design-system/vitest.setup.ts` already carries, for the next `*.spec.ts` that
  mounts a component using it (none in this change does).
- `eslint.config.js`'s Safari-compat `no-restricted-syntax` block now exempts
  `**/*.spec.ts` alongside the existing `**/*.test.ts` — test fixtures never ship
  to a browser.
- No behavior change for accounts without the "Jobs matching my profile" alert
  enabled — `syncProfileAlert` is already a no-op when no profile-derived search
  exists.

**Deliberately not done, flagged for a separate change:**
- **Centralizing the resync in `profileStore` itself** (every mutator already
  funnels through one `save()`) instead of one call at each writer. Now five
  known call sites have needed the same fix by hand across two review passes —
  real evidence a shared choke point would pay for itself — but doing that
  refactor inside a bugfix PR conflates two changes; tracked as a follow-up
  rather than done here.
- **A stub for `$app/environment`** in the new `components` vitest project. Real
  today's `*.spec.ts` files never need it (none renders a component that
  transitively imports it); adding an alias nothing exercises would be
  speculative infrastructure, not a fix for a failure this change hits.

## Capabilities

No requirement text changes: `profile-alert-toggle`'s "Profile-derived search stays
in sync" requirement already mandates this behavior for every profile save. This
closes an implementation gap in one section (Skills) that was missed when that
section moved to its own autosaving tab, not a change in what the system is
supposed to do.

### New Capabilities

(none)

### Modified Capabilities

(none — see above; `skip_specs: true` is set in `.openspec.yaml`)

## Impact

- Frontend only: `web/src/lib/components/profile/SkillsCard.svelte`,
  `web/src/routes/my/profile/skills/+page.svelte`,
  `web/src/lib/profileAlertSync.ts` (new, relocated from `actions.ts`),
  `web/src/routes/my/profile/actions.ts`, `web/src/lib/components/JobMatch.svelte`,
  `web/src/lib/components/ProfileForm.svelte`, `web/src/routes/onboarding/+page.svelte`,
  `web/vitest.setup.ts`, `web/eslint.config.js`.
- No migrations, no API contract change, no other package affected.
