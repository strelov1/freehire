## Context

See `proposal.md` for motivation. Full file-level design was worked out and
approved with the user beforehand at
`docs/superpowers/specs/2026-09-04-profile-section-routes-design.md`; this
document summarizes the decisions in OpenSpec's format and is the source the
task list is built from.

`/my/tracking` and `/my/activity` already implement this exact pattern (a
section `+layout.svelte` with a routed tab strip via `$lib/actions/tablist`,
each view its own leaf `+page.svelte`) — this change is a third application of
that convention, not a new one.

## Goals / Non-Goals

**Goals:**
- Every one of the 8 profile sections gets its own URL under `/my/profile/*`.
- Preserve today's "no profile yet" behavior exactly: the setup form and
  `AccountPreferences` render on any of the 8 URLs, no tab strip, until a
  profile exists.
- Preserve today's visual tab-strip styling (underline + icon), not the
  Tracking/Activity pill style — Profile's styling is a separate, deliberate
  existing convention.
- Keep the very recent `?tab=<id>` links working via a redirect.

**Non-Goals:**
- No change to any section's own component or its save behavior.
- No change to sibling `/my/**` areas.
- No visual redesign of the tab strip.

## Decisions

- **Shared state via the existing singleton-store convention, not Svelte
  context.** `profileStore`/`resumeStore` (`$lib/profile.svelte`,
  `$lib/resume.svelte`) are already app-wide `.svelte.ts` singletons, imported
  directly wherever a `profile`/`resumeMeta` value is needed — no plumbing
  through the layout is needed for those two at all. This codebase has no
  existing use of Svelte's Context API anywhere; introducing it here for the
  first time was rejected in favor of the pattern already used throughout
  (`profileStore`, `resumeStore`, `savedSearches`).
  `+layout.svelte` keeps owning only what's genuinely layout-scoped: the
  initial `status` load gate and the setup-vs-tabs decision. The mutation
  callbacks (`handleSaved`/`syncProfileAlert`/`handleCvUploaded`/
  `handleCvDeleted`) touch only those singleton stores, not any local
  component state, so they become plain functions in a new
  `web/src/routes/my/profile/actions.ts`, imported directly by whichever leaf
  page needs them (Profile, Location) — same "import a shared module"
  shape as the stores, no props drilling and no context.
  Two pieces of state that used to be layout-level lose that scope entirely
  now that each section is its own component instance: `screeningAnswers`
  (read/refreshed only by the Screening section) moves into
  `screening/+page.svelte`; the bank-edit `actionError` banner (only ever
  shown for Experience, per the original code's own comment that it "must
  not keep showing" once the visitor leaves) moves into
  `experience/+page.svelte` as local state — leaving that page naturally
  clears it, no manual `$effect` reset required.
- **Tab strip becomes `<a href>` + `page.url.pathname`, not a client-side
  `goto()` on click.** Matches Tracking/Activity; gives real browser
  navigation (back/forward, open-in-new-tab) for free.
- **Reuse `$lib/actions/tablist` for keyboard behavior, keep existing visual
  classes.** The action only wires roving-tabindex + arrow/Home/End key
  handling; it doesn't impose the pill styling (`routeTabClass` is a separate,
  optional helper this change does not use).
- **`?tab=<id>` compatibility lives in a new `+page.ts` on the index route
  only**, not in the layout — it only matters when the URL has no path
  segment past `/my/profile`, so it belongs on that leaf, not on every route.
- **The 4 existing redirect stubs are deleted outright**, not kept alongside
  the new pages. Confirmed by repo-wide search that nothing else links to
  `/my/profile?tab=<id>` except those very stubs, so there is no second caller
  to preserve.

## Risks / Trade-offs

- [Direct navigation to a leaf route on first load must trigger the same
  client-side data load the layout currently does via `$effect`] →
  `+layout.svelte`'s `$effect`/`isAuthenticated()` load runs on any entry
  point into the layout, not only on the index route, so this is inherent to
  moving the effect up one level, not new risk.
- [A stale bookmark to one of the 4 old pre-consolidation URLs
  (`/my/profile/experience` etc.) already 308-redirects today] → after this
  change those same URLs render directly instead of redirecting, which is
  strictly less indirection, not a break.
- [Adding `my/profile/+layout.svelte` implicitly nests every existing page
  under `my/profile/**` inside it, including `cv-readiness/+page.svelte` — an
  existing, unlisted route that is deliberately not one of the 8 sections and
  must keep rendering only inside `my/+layout.svelte`. Found in code review,
  not caught by `svelte-check` or the manual QA pass since neither exercises
  SvelteKit's layout-inheritance rules.] → renamed to
  `cv-readiness/+page@my.svelte`, the same `@`-reset mechanism
  `my/assistant/+layout@.svelte` already uses, verified live (with a profile)
  to no longer show the section tab strip.

## Migration Plan

Frontend-only, no data migration. Deploy is a single frontend release:
- No feature flag — the old `?tab=` links keep resolving via the compatibility
  redirect, so there is no broken-link window.
- Rollback is a plain revert (no schema/state to unwind).

## Open Questions

None — the pattern is a direct copy of `/my/tracking` and `/my/activity`,
already reviewed and shipped twice in this repo.
