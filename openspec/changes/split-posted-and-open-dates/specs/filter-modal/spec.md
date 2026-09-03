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

The `Posted` entry SHALL belong to the `ROLE` section, adjacent to `Experience`. It
states how current a posting is, which is a property of the posting and not a
requirement placed on the candidate, so `REQUIREMENTS & ELIGIBILITY` — where it sat as
the rail's last entry — both misdescribed it and buried it.

The `Posted` pane SHALL be the modal's single answer to how current a posting is, and
SHALL render, in order: the `Open within` bound, the `Posted within` bound, and the
`reality` facet's three classes as excludable chips. There SHALL NOT be a standalone
`Posting reality` rail entry — the classes and the age bounds answer one question, and
the age is most of what makes a posting read as evergreen, so a separate entry would
split that question across two tabs. The pane's staged count SHALL include both bounds
and any staged reality selection.

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

#### Scenario: The reality facet is reachable from the Posted pane

- **WHEN** the user selects the `Posted` rail entry
- **THEN** the right pane renders the `fresh`, `stale` and `likely-evergreen` chips
  with the facet's Exclude affordance, beneath the two date bounds

#### Scenario: The rail carries no standalone reality entry

- **WHEN** the user opens the modal
- **THEN** no `Posting reality` entry appears in the rail

#### Scenario: The Posted pane counts both bounds and the reality selection

- **WHEN** the user stages an `Open within` bound and excludes `likely-evergreen`
- **THEN** the `Posted` rail entry shows the count `2`

## ADDED Requirements

### Requirement: The Posted pane offers two date bounds, labelled for whose date each is

The `Posted` pane SHALL offer two independent upper bounds over preset stops:

- **Open within** — how long the posting has been in the catalogue, bounding the
  date the system first recorded it. It SHALL be presented first.
- **Posted within** — how recently the source states the posting went up.

Each control SHALL state which date it bounds, because the two disagree precisely on
the postings a reader most wants to exclude: a source that rewrites its posting date
reads as posted today and has been open for months. Neither SHALL be presented as the
authoritative "age" of a posting.

The two SHALL be independent: setting or clearing one SHALL NOT change the other, and
both applied together SHALL narrow the list by both.

The `Open within` control SHALL be revealed only when the runtime feature flag for it
is enabled. Its URL parameter SHALL be honoured whether or not the flag is set, so the
bound can be verified against production before the control is reachable. When the flag
is off, the pane SHALL render the `Posted within` bound and the reality chips as
before, with no gap where the hidden control would sit.

#### Scenario: Both bounds are offered, each labelled for its date

- **WHEN** the user opens the `Posted` pane with the flag enabled
- **THEN** an `Open within` control appears above a `Posted within` control, and each
  names the date it bounds

#### Scenario: The two bounds are independent

- **WHEN** the user sets `Open within` to 30 days and then clears `Posted within`
- **THEN** the `Open within` bound is unchanged and still applied

#### Scenario: Both bounds narrow the list together

- **WHEN** the user sets `Open within` to 30 days and `Posted within` to 3 days
- **THEN** the list is restricted by both bounds

#### Scenario: The open bound is hidden while its flag is off

- **WHEN** the flag is unset, empty, or unrecognized and the user opens the `Posted`
  pane
- **THEN** the `Open within` control is not rendered, and the `Posted within` control
  and reality chips render as before

#### Scenario: A shared open-bound link applies with the flag off

- **WHEN** a link carrying the open bound is opened while the flag is off
- **THEN** the list is restricted by that bound and the parameter is preserved in the
  URL
