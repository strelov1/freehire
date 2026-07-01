## ADDED Requirements

### Requirement: Mobile filters open from a pinned left-edge tab

On viewports below the `md` breakpoint, the jobs list SHALL expose filter access
through an icon-only tab pinned to the left viewport edge, mirroring the swipe
tab on the right. The tab SHALL be present on both the standalone `/jobs` list
and embedded (scoped) job lists, and SHALL be hidden at and above the `md`
breakpoint, where the persistent filters aside is shown instead.

#### Scenario: Left tab opens the filters drawer on mobile
- **WHEN** a user on a sub-`md` viewport taps the left-edge filters tab
- **THEN** the filters drawer opens (the same drawer the previous inline button opened)

#### Scenario: Tab is hidden on desktop
- **WHEN** the viewport is at or above the `md` breakpoint
- **THEN** the left-edge filters tab is not shown and the persistent aside panel provides filter access

### Requirement: Active-filter count shows as a badge on the tab

The number of active filters SHALL be shown as a corner badge on the filters tab
when at least one filter is active, and SHALL be absent when no filters are active.

#### Scenario: Badge reflects active filters
- **WHEN** one or more filters are active
- **THEN** the tab shows a badge with the active-filter count

#### Scenario: No badge when no filters
- **WHEN** no filters are active
- **THEN** the tab shows no count badge

### Requirement: No overlap or empty top offset on the mobile list

The mobile jobs list SHALL NOT render the filters trigger inline in a way that
overlaps the swipe tab, and SHALL NOT add an empty top offset above the job
count. The first content line SHALL remain clear of the left-edge tab on mobile.

#### Scenario: Filters tab and swipe tab do not overlap
- **WHEN** the standalone jobs list is viewed on a sub-`md` viewport
- **THEN** the filters tab (left edge) and the swipe tab (right edge) occupy opposite edges without overlapping, and no empty inline Filters row precedes the job count

#### Scenario: Job count is not covered by the tab
- **WHEN** the job-count line renders on a sub-`md` viewport
- **THEN** it is offset clear of the left-edge tab (and has no such offset at or above `md`)
