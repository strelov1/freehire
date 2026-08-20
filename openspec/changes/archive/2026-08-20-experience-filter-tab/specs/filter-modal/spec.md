## MODIFIED Requirements

### Requirement: The job filters are edited in a two-pane modal grouped into sections

The web client SHALL provide a filter modal opened from an **All filters** control.
The modal SHALL present two panes: a left rail listing every filter facet grouped
under section headings (`ROLE`, `PAY & BENEFITS`, `REQUIREMENTS & ELIGIBILITY`), and
a right pane rendering the controls of the facet selected in the rail. Each rail
entry SHALL show a count of how many values are currently staged for that facet, and
selecting a rail entry SHALL switch the right pane to that facet without closing the
modal or applying anything. Related facets MAY be consolidated into one rail entry
whose pane renders each as its own labelled chip group (e.g. work format + employment
type; industry + company type + collection; salary currency + minimum; English level +
job language; relocation + visa).

The seniority pills SHALL NOT be consolidated into the specialization pane. They
belong to the `Experience` rail entry (see "The rail carries an Experience pane"),
because seniority states how much experience a posting wants, not which role it is.

#### Scenario: Opening the modal shows the sectioned rail and the first facet

- **WHEN** the user activates **All filters**
- **THEN** the modal opens with the facets grouped under `ROLE` / `PAY & BENEFITS` /
  `REQUIREMENTS & ELIGIBILITY` in the left rail and the first facet's controls in the
  right pane

#### Scenario: Selecting a rail entry switches the pane without applying

- **WHEN** the user clicks a different facet in the rail
- **THEN** the right pane renders that facet's controls and no change is applied to
  the job list

#### Scenario: The rail shows staged counts per facet

- **WHEN** two values are staged for a facet (or a consolidated entry)
- **THEN** that facet's rail entry shows the count `2`

#### Scenario: The specialization pane no longer carries seniority

- **WHEN** the user opens the specialization (`Role`) pane
- **THEN** it renders the role picker, the specialization chips and the AI
  specialization facet, and does NOT render the seniority pills

## ADDED Requirements

### Requirement: The rail carries an Experience pane

The filter modal's `ROLE` section SHALL carry an `Experience` rail entry whose pane
renders two labelled controls: the `seniority` pills, and a years-of-experience
control bounding how much experience a posting asks for. The pane SHALL be reachable
by the same rail interaction as every other entry, and its rail entry's staged count
SHALL cover both controls together.

Moving the seniority pills into this pane SHALL NOT change the `seniority` URL
parameter, its allowed values, or its exclusion (`seniority_exclude`) behaviour — a
URL that selects a seniority before this change SHALL select the same seniority
after it.

#### Scenario: The Experience pane renders both controls

- **WHEN** the user selects the `Experience` rail entry
- **THEN** the right pane renders the seniority pills and the years-of-experience
  control

#### Scenario: An existing seniority URL still applies

- **WHEN** a filter URL carrying `seniority=senior` is loaded after the move
- **THEN** the senior pill is selected in the `Experience` pane and the job list is
  filtered to senior postings, exactly as before the move

#### Scenario: The rail count sums the pane's controls

- **WHEN** the user stages one seniority value and a years bound
- **THEN** the `Experience` rail entry shows the count `2`

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
