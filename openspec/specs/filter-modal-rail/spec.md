# filter-modal-rail Specification

## Purpose
The rail's role picker and years-of-experience control, and the rule that every job facet is reachable from it. Split out of filter-modal.

## Requirements

### Requirement: Role is a count-driven picker alongside seniority and specialization

The filter modal SHALL present a "Role" control that lets the user pick one or
more natural roles from the derived `role` facet. The control SHALL be
count-driven (dynamic): its options come from the live facet distribution,
ordered busiest-first, with a facet-local typeahead for high cardinality, reusing
the same dynamic facet-section path as the `skills` control. Each role SHALL
render its catalog label (not the raw slug). The control SHALL support per-facet
Exclude and an AND/OR mode, consistent with the other high-cardinality facets.
The Role control SHALL be **additive**: the
existing seniority and specialization controls remain available and unchanged in
this change.

#### Scenario: Roles are offered busiest-first with labels

- **WHEN** the user opens the Role control
- **THEN** the offered roles come from the live `role` facet distribution ordered
  by descending count, each shown with its catalog label

#### Scenario: Selecting roles filters the results

- **WHEN** the user selects the "Founding Engineer" and "Staff Engineer" roles
  and applies the filters
- **THEN** the search is filtered to jobs whose `roles` include
  `founding_engineer` or `staff_engineer`

#### Scenario: Seniority and specialization controls remain

- **WHEN** the Role control is present in the modal
- **THEN** the existing seniority and specialization controls are still shown and
  usable

### Requirement: Years of experience is a single upper-bound control over preset stops

The years-of-experience control SHALL be a single-handle range input that moves over
an ordered list of preset stops rather than over raw years, following the pattern the
freshness ("Posted within") control already uses. The stops SHALL be, in order: no
experience required, 1, 2, 3, 5, 8, 10 years, and an unbounded **Any**. The control
SHALL default to **Any**.

Each stop SHALL set `experienceYearsMax` to its year value, and **Any** SHALL clear
it. The control SHALL NOT set a lower bound; `experience_years_min` remains an
API-only parameter with its existing floor semantics. The leftmost stop SHALL be
labelled as requiring no prior experience and SHALL send `experience_years_max=0`, so
the entry-level case is a position on this control rather than a separate toggle.

The stops are deliberately non-linear because the catalogue's stated requirements
are: measured on production, postings asking 5–7 years outnumber those asking 8–9 by
more than five to one, so an evenly spaced scale would spend most of its travel on a
thin tail.

#### Scenario: The default sends no parameter

- **WHEN** the user opens the `Experience` pane and leaves the control at **Any**
- **THEN** `experience_years_max` does not appear in the applied filter state or the
  URL

#### Scenario: A stop sets the upper bound

- **WHEN** the user moves the control to the `3` stop and applies
- **THEN** `experience_years_max=3` is present in the applied filter state and the
  URL

#### Scenario: The leftmost stop selects the no-experience postings

- **WHEN** the user moves the control to its leftmost stop and applies
- **THEN** `experience_years_max=0` is applied and only postings stating that no
  prior experience is required are returned

#### Scenario: Returning to Any clears the bound

- **WHEN** the user moves the control back to **Any**
- **THEN** `experience_years_max` is removed from the applied filter state and the URL

### Requirement: The years control states its own coverage

Roughly half the searchable catalogue states no experience requirement at all, and
any bound excludes every posting with no stated value. The pane SHALL therefore
display, alongside the years control, a note that the bound matches only postings
whose experience requirement is stated. The note SHALL be present whenever the
control is rendered, not only once a bound is set — a user who cannot see why the
result count collapsed has already been misled.

#### Scenario: The coverage note is always visible

- **WHEN** the `Experience` pane is rendered, with the control at **Any** or at any
  bounded stop
- **THEN** the note stating that the bound matches only postings with a stated
  experience requirement is visible

### Requirement: Every job facet is reachable from the rail

Every facet declared in the job filter vocabulary SHALL either have a rail entry
of its own or be named in a documented exception list, and this SHALL be enforced
by a test. An exception SHALL exist only where the facet's controls are rendered
inside another entry's pane, and SHALL record which pane hosts it.

The company catalog's rail already carries this guarantee. The job rail did not,
and the `reality` facet was declared, served, and unreachable in the interface as
a result — a facet that is defined but has no entry fails silently, in the one
direction no test was watching.

#### Scenario: A facet with no entry and no exception fails the test

- **WHEN** a facet is added to the job filter vocabulary with neither a rail entry
  nor an exception-list membership
- **THEN** the rail completeness test fails

#### Scenario: A facet hosted inside another pane is a documented exception

- **WHEN** a facet's controls are rendered inside another entry's pane
- **THEN** it appears on the exception list with the hosting pane recorded, and
  the test passes
