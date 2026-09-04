## Why

`/my/profile`'s 8 sections (Profile, Contacts, Location, Skills, Experience,
Education, Screening answers, Settings) are switched by local component state
(`view`), not the URL. None of them is linkable, bookmarkable, or survives a
reload except the default "Profile" one. Four of the eight used to be their own
routes and were recently collapsed into `?tab=<id>` with 308 redirect stubs at
their old paths — this change reverses that for all eight, following the pattern
already shipped for `/my/tracking` and `/my/activity` (a section `+layout.svelte`
holding a routed tab strip via `$lib/actions/tablist`, with each view as its own
leaf route).

## What Changes

- Add a real route per section: `/my/profile` (Profile, the index), `/contacts`,
  `/location`, `/skills`, `/experience`, `/education`, `/screening`, `/settings`
  (new — Settings/AccountPreferences did not have its own route before).
- Add `web/src/routes/my/profile/+layout.svelte`: owns the shared profile/résumé
  load, the "no profile yet" setup gate (unchanged behavior — suppresses the tab
  strip and always shows the setup form regardless of which route was requested),
  and the tab strip itself, now `<a href>` elements keyed off `page.url.pathname`
  instead of buttons toggling local state. Keyboard roving-tabindex reuses
  `$lib/actions/tablist`, replacing the page's hand-rolled arrow-key handler.
- **BREAKING (internal only)**: remove the `?tab=<id>` query-param mechanism.
  `contacts/+page.ts`, `experience/+page.ts`, `screening/+page.ts`,
  `skills/+page.ts` (today's 308-redirect stubs to `/my/profile?tab=<id>`) are
  deleted and replaced by real `+page.svelte` files. A new `+page.ts` on the bare
  `/my/profile` route 308-redirects `?tab=<id>` to `/my/profile/<id>`, so the
  very recently shared query-param links keep working.
- No visual redesign: the tab strip keeps its current underline + icon styling.
  No change to any section's own component (`ProfileForm`,
  `CandidateContactsEditor`, `LocationCard`, `SkillsCard`, `ExperienceBankView`,
  `EducationCard`, `ScreeningAnswersForm`, `AccountPreferences`).

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `search-profiles`: adds a new requirement that the profile view's sections are
  each addressable by their own URL (deep-linkable, bookmarkable, survive a
  reload), on top of the existing "Profile management UI" requirement's
  description of what the view shows — this change does not alter what any
  section does or how it saves, only how each is reached.

## Impact

- Frontend only, `web/src/routes/my/profile/**`: new `+layout.svelte`; new
  `+page.svelte` under `contacts/`, `location/`, `skills/`, `experience/`,
  `education/`, `screening/`, `settings/`; the existing `+page.svelte` narrows to
  the Profile section only; a new `+page.ts` on the index route for `?tab=`
  compatibility; the four existing `contacts|experience|screening|skills
  /+page.ts` redirect stubs are deleted.
- `cv-readiness/+page.svelte` (an existing, unlisted route under
  `/my/profile/` that is deliberately not one of the 8 sections) is renamed to
  `cv-readiness/+page@my.svelte` — SvelteKit nests every page inside every
  ancestor layout by default, so without this reset the new
  `my/profile/+layout.svelte` would wrap it too (its tab strip and its "no
  profile yet" gate both being wrong there). The `@my` reset keeps it exactly
  where it was, inside `my/+layout.svelte` only — same mechanism already used
  by `my/assistant/+layout@.svelte`.
- No backend/API changes, no migration, no other route or package affected.
