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

The `Posted` (freshness) entry SHALL belong to the `ROLE` section, adjacent to
`Experience`. It states how old a posting is, which is a property of the posting
and not a requirement placed on the candidate, so `REQUIREMENTS & ELIGIBILITY` —
where it sat as the rail's last entry — both misdescribed it and buried it.

The rail SHALL carry a `Posting reality` entry rendering the `reality` facet's
three classes as excludable chips. The facet was defined and served but had no
rail entry, making it reachable only by hand-editing the URL.

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

#### Scenario: Freshness sits beside Experience under ROLE

- **WHEN** the user opens the modal
- **THEN** the `Posted` entry appears in the `ROLE` section adjacent to
  `Experience`, and not under `REQUIREMENTS & ELIGIBILITY`

#### Scenario: The reality facet is reachable from the rail

- **WHEN** the user selects the `Posting reality` rail entry
- **THEN** the right pane renders the `fresh`, `stale` and `likely-evergreen`
  chips with the facet's Exclude affordance

## ADDED Requirements

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
