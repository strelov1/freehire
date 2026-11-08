# Profile Skills tab

## Problem

On `/my/profile`, the "Skills" and "Skills to avoid" fields live inside the Settings
tab's "Skills & role" form section, alongside Role/specializations, CV upload, and
headshot. They deserve their own top-level tab.

## Scope

Split Skills / Skills to avoid out into a dedicated top-level page tab, editable
independently of the rest of the profile form (no shared "Save changes" button).

Out of scope: changing the Role/specializations field, the CV upload flow's storage,
or the `user_profiles` schema. No backend changes — the same `PUT /me/profile`
endpoint is called, just from a different place on the frontend.

## Design

### Tab placement

`web/src/routes/my/profile/+page.svelte`: add `{ id: 'skills', label: 'Skills' }` to
`TABS`, right after `settings`. Like the other data tabs, it is only reachable once a
profile exists (`profile !== null`) — during first-time set-up there are no top-level
tabs at all, only the bare `ProfileForm`.

### `ProfileForm.svelte`

The Skills / Skills to avoid blocks currently in the `main` form-tab stay there **only
while creating a profile** (`profile === null`, i.e. `!editing`) — the server requires
at least one skill to create a profile, and the new Skills tab isn't reachable yet at
that point. Once a profile exists (`editing`), those two blocks are removed from
`ProfileForm`; the `main` form-tab's label changes from "Skills & role" to "Role".

CV upload (`analyzeResume`):

- `!editing` (creation flow): unchanged — extracted skills merge into the local
  `skills` buffer, persisted together with Role on the next explicit Save.
- `editing`: extracted skills are written straight to the profile via
  `profileStore.addSkills(cv.skills)` (one PUT), not buffered locally. The existing
  "Added N skills from your CV" note stays, wording adjusted to point at the Skills tab
  instead of "below".

### `profileStore` (`web/src/lib/profile.svelte.ts`)

New method:

```ts
addSkills(skills: string[]): Promise<UserProfile>
```

Folds `withSkill` (from `profileSkills.ts`) over each new skill and issues a single
PUT — the bulk counterpart to the existing one-at-a-time `addSkill`. Queued through the
same `#queue` as the other mutators so it can't race a manual toggle made from the new
Skills tab.

### `SkillsView.svelte` (new)

`web/src/lib/components/SkillsView.svelte`, modeled on `ExperienceBankView` /
`VerdictView`. No local buffered skill state — reads `profileStore.profile?.skills` /
`?.excluded_skills` reactively, same as `JobMatch.svelte`'s existing `avoided` derived
set. Renders the same two `RemoteSearchSelect` blocks (counts, chip styling, copy) that
exist today in `ProfileForm`.

Toggling a chip calls `profileStore.addSkill` / `removeSkill` / `avoidSkill` /
`unavoidSkill` directly — writes go out immediately, no Save button. Error/pending
handling mirrors `JobMatch.svelte`: one `pending` boolean disables both controls while
a write is in flight; a `failed: string | null` renders
`Could not update {failed} in your profile. Try again.` on rejection.

### Shared skill-dictionary loader

Both `ProfileForm` and `SkillsView` need the typeahead's skill universe (currently
`ProfileForm.loadSkills()`: `api.facetCounts` → sort by count). Extract this into
`web/src/lib/skillDictionary.ts`, exporting `loadSkillDistribution(): Promise<FacetOption[]>`,
so the fetch/sort logic isn't duplicated between the two components.

## Error handling

No new error-handling design needed — reuses the `JobMatch.svelte` pattern verbatim
(try/catch per mutator call, `failed` state, generic per-skill message).

## Testing

No backend changes, so no Go-side tests. No frontend unit-test runner in this repo
(vitest config edits are inert here). Manual verification via dev server:

1. Create a profile from scratch — Skills and Role still required together, form
   behaves as today.
2. Edit an existing profile — toggle a skill and an avoided skill on the new Skills
   tab; confirm each persists immediately with no Save click, and survives a reload.
3. Upload a CV against an existing profile — confirm extracted skills land in the
   profile automatically (visible on the Skills tab) without touching Save.
