## ADDED Requirements

### Requirement: Every facet control shows a live match count

Every facet control in the filter modal SHALL display a match count per option —
the seniority pills, the specialization chips, and every other chip/pill and
location control — the same way the role picker and the dynamic selects already
do. A count SHALL be omitted only when the distribution has no entry for that
control (e.g. the endpoint is unavailable), never breaking the control.

#### Scenario: Seniority and specialization show counts

- **WHEN** the modal's Role pane is open with facet counts available
- **THEN** each Seniority pill and each Specialization chip shows its job count
  beside its label

### Requirement: Counts recompute live from the staged selection

The modal's option counts SHALL be sourced from the **staged** (in-progress)
selection, recomputed (debounced) as the user toggles options, using the
**disjunctive** distribution — so a facet keeps showing its sibling counts while
selected. The deferred-apply contract is unchanged: recomputing counts SHALL NOT
apply the filter to the live list; only the footer Apply action does.

#### Scenario: Toggling an option updates the other facets' counts

- **WHEN** the user toggles a facet option in the modal
- **THEN** the counts on the other facets recompute from the new staged selection
  (after a short debounce), while the live job list stays unchanged until Apply

#### Scenario: The edited facet still shows its alternatives

- **WHEN** the user selects one value of a facet
- **THEN** that facet's other values still show their (disjunctive) counts, so the
  user can see and switch to alternatives

### Requirement: The preview shows a loading state during recompute

The modal's "Show N results" footer SHALL show a loading indicator instead of a
stale number while a staged-count recompute is in flight, and the option counts
MAY be dimmed; when the recompute resolves, the number (and counts) SHALL appear.

#### Scenario: Spinner while recomputing

- **WHEN** the user toggles an option and the staged-count fetch is in flight
- **THEN** the "Show N results" button shows a spinner rather than the previous
  number
- **AND** once the fetch resolves, the button shows the updated job count
