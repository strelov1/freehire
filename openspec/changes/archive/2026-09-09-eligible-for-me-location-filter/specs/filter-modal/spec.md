## ADDED Requirements

### Requirement: The Location pane offers an "Eligible for me" pill

The Location pane SHALL show an "Eligible for me" pill alongside the existing
`Global` / `Not specified` flat pills, above the region → country tree. The pill
SHALL be a single toggle (on/off, not a cycling include/exclude/off chip like the
region and country chips) reflecting whether the filter described by the
`eligible-for-me-filter` capability is currently staged. When the underlying
geography source is unavailable, the pill SHALL render in a disabled state rather
than being hidden, so a visitor understands the option exists but has nothing to
turn it on with.

#### Scenario: The pill sits with the other flat pills

- **WHEN** the Location pane is shown and a geography source is available
- **THEN** the "Eligible for me" pill is shown alongside `Global` and
  `Not specified`, interactable as one on/off toggle

#### Scenario: The pill is disabled when no geography source is available

- **WHEN** the Location pane is shown and neither the profile country nor the
  edge-derived region resolves to a value
- **THEN** the "Eligible for me" pill is shown disabled

#### Scenario: Turning the pill on reflects in the selected-location chips

- **WHEN** the visitor turns the pill on
- **THEN** the resulting staged country/region and the `Global`/`Not specified`
  values appear in the pane's selected-location chips row, each individually
  removable like any other selection
