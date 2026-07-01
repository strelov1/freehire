## Why

On the mobile jobs list the inline "Filters" pill (rendered in the content flow,
right-aligned) sits at the same vertical level as the fixed swipe-mode tab pinned
to the right viewport edge, so the two visually overlap. The inline pill's row
also pushes the job list down with an empty top offset (on the standalone list it
contains only a spacer). The result is a cramped, overlapping header area.

## What Changes

- Turn the mobile "Filters" trigger into an icon-only tab pinned to the **left**
  viewport edge, mirroring the swipe tab on the right (symmetric edge tabs, no
  overlap). The active-filter count moves from inline `(N)` text to a corner badge.
- Remove the now-empty inline Filters row on the standalone list, eliminating the
  top offset above the job count. The embedded (company) view keeps its inline
  search input but drops the redundant inline Filters button in favour of the tab.
- Nudge the first content line (the job count) clear of the left tab on mobile so
  the tab doesn't cover it; desktop is unaffected (the aside panel is unchanged).

## Capabilities

### New Capabilities
- `mobile-filter-access`: How mobile users open the filters panel on the jobs list —
  the pinned left-edge tab, its active-count badge, and its coexistence with the
  swipe tab and page content.

### Modified Capabilities
<!-- No existing spec's requirements change. -->

## Impact

- `web/src/lib/components/JobsView.svelte` (markup + Tailwind classes only; no data,
  API, or reactive-state changes).
- Desktop layout, the filters drawer behaviour, and the swipe tab are unchanged.
