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

- **Shared state via Svelte context, not props drilling or per-page refetch.**
  `+layout.svelte` owns `profileStore`/`resumeStore` loading, `screeningAnswers`,
  `actionError`, and the mutation callbacks (`handleSaved`, `handleCvUploaded`,
  `handleCvDeleted`, `offerRefreshAfterBankEdit`), and exposes them to leaf pages
  via `setContext`. Alternative considered: give each leaf page its own
  `+page.ts`/`+page.svelte` load logic — rejected, since the shared profile/CV
  data is read by most sections and refetching it per navigation would be both
  slower and a behavior change (today it loads once).
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

## Migration Plan

Frontend-only, no data migration. Deploy is a single frontend release:
- No feature flag — the old `?tab=` links keep resolving via the compatibility
  redirect, so there is no broken-link window.
- Rollback is a plain revert (no schema/state to unwind).

## Open Questions

None — the pattern is a direct copy of `/my/tracking` and `/my/activity`,
already reviewed and shipped twice in this repo.
