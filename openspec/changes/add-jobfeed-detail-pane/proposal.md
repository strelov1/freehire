## Why

Clicking a job on the home feed navigates away to a full `/jobs/[slug]` page, losing the feed's scroll position and filter state — every job costs a full round trip back to the list. Reworking the feed into a compact list + inline detail pane lets a user browse many jobs in one place, the way `/tailor/[slug]`'s chat+artifact split already works for CV tailoring. A design spike (2026-08-06) confirmed the content side of this is low-risk: `MatchAnalysisFull` already renders its full report (verdict gauge, dimensions, ATS requirements, strengths/gaps) cleanly at this layout's width, proven in production at an even narrower 340–720px in `ArtifactPanel.svelte`'s `stacked` mode.

## What Changes

- The home feed becomes a two-column desktop layout: a compact job list on the left (`JobRow`'s existing `compact` presentation) and a detail pane on the right showing the selected job's full detail — the same components `/jobs/[slug]` renders today — in place, no navigation away.
- `/jobs/[slug]`'s URL and content are unchanged; it becomes the thing rendered *inside* the pane by sharing a layout with the feed (a SvelteKit route group), rather than a new duplicate detail route. Deep links and shares keep working exactly as before.
- The feed's filters move from the current left `<aside>` sidebar to a horizontal bar docked above the list column — closer to the reference layout that prompted this change, and the natural place for them once the sidebar's width is needed for the list rail.
- Below the desktop breakpoint, the pane disappears; the feed is list-only and a card click navigates to `/jobs/[slug]` as a full page, same as it does today.
- **BREAKING** (behavior, not URLs): on desktop, clicking a feed card no longer leaves the list by default — it opens inline instead.

## Capabilities

### New Capabilities
- `job-feed-detail-pane`: the home feed's compact-list-plus-inline-detail-pane browsing behavior on desktop, including the relocated horizontal filter bar and the mobile list-only fallback.

### Modified Capabilities
(none — no existing spec file documents the feed's current click-to-navigate interaction, so there is nothing to diff against; this proposal documents new frontend behavior even though it changes code on an existing route)

## Impact

- `web/src/routes/+page.svelte` / `+page.server.ts` (home feed) and `JobsView.svelte` change materially: new two-column layout, filter bar relocation, selection state.
- `web/src/routes/jobs/[slug]/+page.svelte` / `+page.server.ts` move under a shared layout with the feed (route-group restructuring); their own content and data-loading contract are unchanged.
- Sibling routes `web/src/routes/jobs/[slug]/fit`, `.../copies`, `.../discussion`, `.../og.png` need an explicit decision on whether they join the shared layout too, or stay outside it (see design.md).
- `JobRow.svelte`'s existing `compact` prop is reused for the list column, not forked.
- The pane reuses `JobView`/`JobMatch`/`MatchAnalysisFull` (`stacked`)/`JobDescription`/`JobApplyForm` unchanged.
- No backend/API changes anticipated; no design-system changes anticipated.
