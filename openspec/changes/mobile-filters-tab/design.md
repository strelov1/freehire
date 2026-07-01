## Context

`JobsView.svelte` renders the jobs list in two modes: `standalone` (the `/jobs`
page, whose text search lives in the app header) and embedded/scoped (e.g. a
company page, which keeps its own inline search input). Mobile filter access is a
button that opens the `drawerOpen` filters drawer; desktop shows a persistent
`aside` panel (`hidden md:block`).

Today the mobile Filters trigger is an inline pill inside a `mb-4 flex` row. On
the standalone list that row holds only a `flex-1` spacer plus the pill, so it
adds an empty top offset and the pill overlaps the fixed swipe tab (`fixed
right-0 top-20`) which is level with it.

## Goals / Non-Goals

**Goals:**
- Remove the Filters-vs-swipe overlap on mobile.
- Remove the empty top offset above the standalone list's job count.
- Keep filter access one tap away on both standalone and embedded views.

**Non-Goals:**
- No change to the drawer contents, the desktop aside, the swipe tab, or any
  data/reactive-state/API behaviour. Markup + Tailwind classes only.

## Decisions

- **Left-edge tab mirrors the swipe tab.** New icon-only button `fixed left-0
  top-20 z-30 ... rounded-r-lg border-l-0 ... md:hidden`, a mirror of the swipe
  tab's `right-0 ... rounded-l-lg border-r-0`. Uses lucide `SlidersHorizontal`.
  Rendered unconditionally (not gated on `standalone`) since both views need
  mobile filter access; `md:hidden` keeps it off desktop where the aside shows.
- **Active count as a corner badge.** `filters.active` moves from inline `(N)`
  text to an absolutely-positioned `bg-primary` badge on the tab, only when `> 0`.
- **Drop the inline row on standalone.** The `mb-4 flex` row now renders only for
  the embedded view (holding its inline search `Input`), and its inline Filters
  button is removed everywhere (replaced by the tab). This kills the top offset.
- **Clear the first line under the tab.** The job-count `<p>` gets `pl-12
  md:pl-0` so the left tab (≈`p-3` + 20px icon) doesn't cover it on mobile; job
  cards start below the tab and need no offset.

## Risks / Trade-offs

- The left tab overlays the extreme left edge of content while scrolling (same
  trade-off already accepted for the right swipe tab). Mitigated for the only
  top-aligned left content (the count) via `pl-12`.
- No web unit-test runner exists; verification is `svelte-check` + a headless
  mobile-viewport screenshot rather than an automated test.
