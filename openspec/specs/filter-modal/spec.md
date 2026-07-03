# filter-modal Specification

## Purpose

The job-search filters are edited in a two-pane modal ("All filters") that groups
facets into sections and gives each facet room for hierarchy and search, while the
sidebar shows only what is currently selected. Selections in the modal are deferred and
applied on a single **Show results** action; the sidebar edits apply immediately.

## Requirements

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
job language; relocation + visa; seniority within specialization).

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

### Requirement: Modal selections are deferred and applied on Show results

The modal SHALL edit a **staged** copy of the filters, seeded from the currently
applied filters when it opens. Toggling any control SHALL update only the staged copy
and SHALL NOT change the job list. A primary **Show results** button SHALL display a
live count of the jobs matching the staged filters and, when activated, SHALL apply
the staged filters to the live (URL-synced) filter state and close the modal.
Dismissing the modal without activating **Show results** SHALL discard the staged
changes.

#### Scenario: Toggling a control does not change the list

- **WHEN** the user toggles a facet value inside the modal
- **THEN** the staged count and the Show-results count update, but the underlying job
  list is unchanged

#### Scenario: Show results applies staged filters

- **WHEN** the user has staged changes and activates **Show results**
- **THEN** the staged filters become the live filter state (reflected in the URL) and
  the modal closes

#### Scenario: Dismissing discards staged changes

- **WHEN** the user stages changes and dismisses the modal (close button, backdrop, or
  Escape)
- **THEN** the live filter state is unchanged from before the modal opened

#### Scenario: Reopening seeds from applied filters

- **WHEN** the user applies filters, then reopens the modal
- **THEN** the staged state reflects exactly the applied filters

### Requirement: The sidebar is a summary of the selected filters

The filter sidebar SHALL display only the currently applied filters, as chips grouped
by facet, alongside the **All filters** button (carrying a badge with the total number
of active filter values) and a **Reset all** control. Removing a chip SHALL apply
immediately to the live filter state (the sidebar edits live state directly; only the
modal defers). With no filters applied, the sidebar SHALL show an empty state rather
than facet controls. The sidebar SHALL also retain the saved-search controls
(save/apply/subscribe).

#### Scenario: Applied filters render as grouped chips

- **WHEN** filters are applied across several facets
- **THEN** the sidebar shows a chip per applied value, grouped under its facet label

#### Scenario: Removing a chip applies immediately

- **WHEN** the user removes a chip from the sidebar
- **THEN** that value is dropped from the live filter state and the job list refreshes
  without opening the modal

#### Scenario: The All-filters badge counts active values

- **WHEN** five filter values are applied
- **THEN** the **All filters** badge shows `5`

### Requirement: Filter options are selected as chips with per-facet Exclude and Clear

Every multi-value facet control in the modal SHALL render its options as chips (the
shared pill primitive), not checkboxes or radio buttons. A selected chip SHALL use the
active (filled) style and an unselected chip the inactive style. Each facet with a
selection SHALL offer a Clear control, and each excludable facet SHALL offer an Exclude
toggle that switches the facet to exclusion (matching the sidebar's per-facet controls).

#### Scenario: Options render as chips

- **WHEN** a facet with a fixed option set is shown in the modal
- **THEN** its options render as chips, and a selected option shows the active chip
  style

#### Scenario: A facet with a selection offers Exclude and Clear

- **WHEN** an excludable facet has at least one selected value
- **THEN** an Exclude toggle and a Clear control are shown for that facet, and Exclude
  switches the selection to the excluded (struck-through) style

### Requirement: Specialization is grouped into collapsible sections

The Specialization (category) facet SHALL group its values under curated section
headings (Engineering; Data & AI; Quality & Security; Design; Product & Management;
Go-to-market & Support) derived from a static category→section map covering the full
category vocabulary. Each section SHALL be collapsible. A facet-local search SHALL
filter the visible options.

#### Scenario: Categories appear under their section

- **WHEN** the Specialization facet is opened
- **THEN** each category value appears as a chip under its section heading, and every
  category vocabulary value belongs to exactly one section

#### Scenario: A section collapses

- **WHEN** the user collapses a section
- **THEN** that section's chips are hidden and its heading remains

### Requirement: Location is a region → country tree with a searchable cities list

The Location facet SHALL present geography as a region → country chip tree plus a flat,
searchable Cities section. Regions SHALL always be shown (the curated macro-regions),
each expandable to the countries mapped to it via the exported country→region map,
scoped to the countries that currently have jobs (plus any already-selected country).
Cities SHALL be a single flat list ordered by job count with a search box, since the
cities facet is an open vocabulary with no reliable country link — city nesting under
countries is out of scope. Selecting a chip at any level SHALL stage that geographic
value using the existing `regions` / `countries` / `cities` filter params. A region
SHALL show its result count. A selected geography value SHALL remain visible (and
deselectable) in the pane even when its current count is zero.

#### Scenario: A region expands to its countries

- **WHEN** the user expands a region
- **THEN** the countries mapped to that region (with jobs) are shown as chips

#### Scenario: Cities are a flat searchable list

- **WHEN** the Location pane is shown
- **THEN** the busiest cities are shown as chips and the rest are reachable by typing in
  the search box, not nested under countries

#### Scenario: Selecting a city stages the city filter

- **WHEN** the user selects a city chip
- **THEN** that city is staged under the `cities` filter param, independent of any
  region or country selection

#### Scenario: The pane is never empty

- **WHEN** the Location pane opens before the facet counts have loaded
- **THEN** the macro-regions are still shown so a location can be selected

### Requirement: Salary and currency are one facet

The modal SHALL present Salary and Currency as a single **Salary** facet: the currency
options as chips and the minimum-annual-salary as a slider, in one pane. Its rail count
SHALL reflect the combined selection (chosen currencies plus a non-zero minimum).

#### Scenario: The Salary pane holds currency and minimum salary

- **WHEN** the user opens the Salary facet
- **THEN** the pane shows currency chips and a minimum-salary slider together

#### Scenario: The rail count combines currency and salary

- **WHEN** one currency is selected and a minimum salary is set
- **THEN** the Salary rail entry shows the count `2`

### Requirement: High-cardinality facets offer a facet-local search

A facet with a large or open option set (e.g. Skills) SHALL provide a search box that
filters its options by label. Selected values SHALL remain visible (pinned) regardless
of the current search text. In the modal's roomy panes such lists SHALL show their full
height rather than a compact scroll cap.

#### Scenario: Searching filters the options

- **WHEN** the user types into a high-cardinality facet's search box
- **THEN** only options whose label matches the query are shown

#### Scenario: Selected values stay visible while searching

- **WHEN** a value is selected and the search query excludes its label
- **THEN** the selected value remains shown as a selected chip

### Requirement: The modal is usable on small screens

On small (mobile) viewports the modal SHALL occupy the full screen and its rail and
pane SHALL adapt (stack or collapse) so every facet remains reachable and applying
stays a single **Show results** action.

#### Scenario: Full-screen modal on mobile

- **WHEN** the modal is opened on a small viewport
- **THEN** it fills the screen, every facet is reachable, and **Show results** applies
  and closes
