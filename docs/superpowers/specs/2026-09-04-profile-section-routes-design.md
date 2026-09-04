# Profile section routes

## Problem

`/my/profile` renders 8 sections (Profile, Contacts, Location, Skills, Experience,
Education, Screening answers, Settings) as a single-page tab strip driven by local
`$state` (`view`). None of them has a real URL: bookmarking, sharing a deep link, or
reloading on a section other than "Profile" all land on the same generic page.

Four of the eight (Contacts, Experience, Screening, Skills) used to be their own
routes; a recent change collapsed them into `?tab=<id>` on `/my/profile` with 308
redirect stubs at their old paths, specifically so existing bookmarks/links kept
working. This spec reverses that consolidation for all eight sections, following the
pattern already established by `/my/tracking` and `/my/activity`.

## Non-goals

- No visual redesign. The tab strip keeps its current underline + icon styling.
- No change to any of the 8 section components themselves (`ProfileForm`,
  `CandidateContactsEditor`, `LocationCard`, `SkillsCard`, `ExperienceBankView`,
  `EducationCard`, `ScreeningAnswersForm`, `AccountPreferences`).
- No change to sibling `/my/**` sections (Tracking, Activity, Inbox, etc).

## Design

### Route layout

```
web/src/routes/my/profile/
  +layout.svelte        # NEW — shared data, setup gate, tab strip, context
  +page.ts              # NEW — ?tab= compat redirect (see below)
  +page.svelte           # Profile view only (was: profile===null setup ∪ all 8 views)
  contacts/+page.svelte  # NEW (replaces contacts/+page.ts redirect)
  location/+page.svelte  # NEW
  skills/+page.svelte    # NEW (replaces skills/+page.ts redirect)
  experience/+page.svelte # NEW (replaces experience/+page.ts redirect)
  education/+page.svelte # NEW
  screening/+page.svelte # NEW (replaces screening/+page.ts redirect)
  settings/+page.svelte  # NEW
  cv-readiness/          # unchanged — already its own route, not one of the 8 tabs
```

`contacts/+page.ts`, `experience/+page.ts`, `screening/+page.ts`, `skills/+page.ts`
(the four redirect stubs) are deleted.

### `+layout.svelte`

Owns everything that today lives at module scope in `+page.svelte` except the
per-view markup:

- `profileStore.ensureLoaded()` / `resumeStore.ensureLoaded()` / screening-answers
  fetch, and the `status` (`loading`/`error`/`ready`) they drive.
- The profile-not-set-up branch: when `profileStore.profile === null`, render only
  `ProfileForm` (profile=null) + `AccountPreferences`, **regardless of which child
  route was requested** — matches today's behavior where the tab strip itself
  doesn't exist until a profile is created.
- `actionError`, `handleSaved`, `syncProfileAlert`, `handleCvUploaded`,
  `handleCvDeleted`, `offerRefreshAfterBankEdit` — unchanged logic, relocated.
- The tab strip: 8 `<a href>` elements (was `<button onclick>`), `aria-selected`
  from `page.url.pathname === href` (index route) or `.startsWith(href + '/')` is
  not needed since these are leaf routes — exact match only, mirroring
  `tracking/+layout.svelte`'s `boardActive`/`listActive` pattern. Keyboard
  roving-tabindex via `use:tablist={path}` (`$lib/actions/tablist`), replacing the
  hand-rolled `handleTabKeydown`. Visual classes unchanged (same underline/icon
  treatment, not `routeTabClass` — that's Tracking/Activity's pill style, a
  deliberately different existing convention noted in the current code comment).
- Exposes shared data to child pages via `setContext` (see below), and renders
  `{@render children()}` inside the existing `role="tabpanel"` wrapper.

### Context contract

A single context key (`profile-section` or similar) carrying a reactive object:

```ts
{
  get profile(): Profile | null;
  get resumeMeta(): ResumeMeta | null;
  get screeningAnswers(): Answers | null;
  get actionError(): string | null;
  handleSaved(): void;
  handleCvUploaded(): void;
  handleCvDeleted(): void;
  offerRefreshAfterBankEdit(): void;
  reloadScreeningAnswers(): Promise<void>;
}
```

Each leaf page reads only the pieces it needs (e.g. `contacts/+page.svelte` reads
`resumeMeta` and `handleSaved`; `experience/+page.svelte` reads only
`offerRefreshAfterBankEdit`).

### `?tab=` compatibility

`+page.ts` (new, on the bare `/my/profile` route only):

```ts
import { redirect } from '@sveltejs/kit';

const KNOWN = ['contacts', 'location', 'skills', 'experience', 'education', 'screening', 'settings'];

export function load({ url }) {
  const tab = url.searchParams.get('tab');
  if (tab && KNOWN.includes(tab)) redirect(308, `/my/profile/${tab}`);
}
```

This is the only place `?tab=` is read going forward; the four old per-section
redirect stubs are removed outright since nothing else in the app links to
`/my/profile?tab=X` (confirmed by grep — the stubs were the only source of those
links).

### Testing

- `svelte-check` must pass (no orphaned imports/types after the split).
- Manual verification in the running dev stack: each of the 8 URLs loads directly
  (fresh navigation, not just client-side tab click), the setup-gate branch still
  suppresses the tab strip when `profile === null`, and `?tab=experience` still
  redirects correctly.
- No existing `.test.ts` file exercises the current tab-switching logic (checked:
  none under `routes/my/profile/`), so no test needs updating for the mechanism
  itself — only added coverage is the manual pass above.

## Open questions

None — the pattern is a direct copy of `/my/tracking` and `/my/activity`,
already reviewed and shipped twice.
