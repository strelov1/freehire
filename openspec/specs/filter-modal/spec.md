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
than facet controls. The saved-search ("My filters") controls SHALL NOT live in the
sidebar; they are presented as a tab inside the modal (see the saved-searches
capability).

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

#### Scenario: The sidebar no longer hosts saved searches

- **WHEN** the filters sidebar renders
- **THEN** it shows only the applied-filter chips, the **All filters** button, and
  **Reset all** — the "My filters" control is not in the sidebar

### Requirement: Filter options are selected as chips with per-facet Exclude and Clear

Every multi-value facet control in the modal SHALL render its options as chips (the
shared pill primitive), not checkboxes or radio buttons, and SHALL let a single facet
hold included and excluded values at the same time. Each facet value carries one of
three states — unselected, included, or excluded — held in the facet's separate
`include` and `exclude` sets. A pills facet SHALL cycle a value through
unselected → included → excluded → unselected on successive activations, showing the
active (filled) style when included and the destructive (red) style when excluded. A
select (searchable) facet SHALL add a picked value to the include set, render each
selected value as a chip carrying a control that toggles it between include and
exclude, and group excluded chips under the destructive style. Included values within
a facet SHALL be ORed by default, with an optional match-all (AND) toggle shown once
more than one value is included; excluded values SHALL always be ANDed (a job matches
only if it has none of them). Each facet with a selection SHALL offer a Clear control
that resets both sets. These per-value controls SHALL match the sidebar's facet
controls, and the whole-facet Exclude toggle SHALL be removed.

#### Scenario: Options render as chips

- **WHEN** a facet with a fixed option set is shown in the modal
- **THEN** its options render as chips, and an included option shows the active chip
  style while an excluded option shows the destructive (red) style

#### Scenario: A pills value cycles through include and exclude

- **WHEN** the user activates the same pills-facet value three times in a row
- **THEN** the value moves unselected → included → excluded → unselected, and the chip
  style tracks each state

#### Scenario: A select value toggles between include and exclude

- **WHEN** a select facet has a value staged in its include set and the user activates
  that chip's include/exclude toggle
- **THEN** the value moves from the include set to the exclude set (and renders under
  the destructive style), without being removed from the facet

#### Scenario: Include and exclude coexist in one facet

- **WHEN** the user includes one value and excludes another in the same facet (e.g.
  include `nodejs`, exclude `php`)
- **THEN** both selections are retained and serialize to `?<param>=nodejs` and
  `?<param>_exclude=php` in the same request

#### Scenario: Match-all toggle applies to included values only

- **WHEN** a facet has two or more included values
- **THEN** a match-all (AND) toggle is shown for the include set, and excluded values
  are unaffected by it (always ANDed)

#### Scenario: Clear resets both sets

- **WHEN** a facet has both included and excluded values and the user activates Clear
- **THEN** the facet's include and exclude sets are both emptied

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

### Requirement: My filters is a deferred tab inside the modal

The modal SHALL present the saved-search ("My filters") control as a rail entry (the
first entry, under a `SAVED` section) so it is reachable wherever the modal opens,
including on mobile viewports where the sidebar is hidden. Selecting the entry SHALL
render the saved-search control in the right pane. Because the modal is deferred,
the control SHALL operate on the **staged** filters: selecting a saved set seeds the
staged filters (previewed, not yet applied) and saving persists the staged filters;
nothing commits to the live filter state until the footer **Show results** action.
The "My filters" tab SHALL be omitted when the modal is opened for a restricted facet
subset that has no saved-search context (e.g. the profile comparison modal).

#### Scenario: My filters is reachable on mobile

- **WHEN** a user opens the modal on a sub-`md` viewport and selects the **My filters**
  rail entry
- **THEN** the saved-search control renders in the pane, without a desktop sidebar

#### Scenario: Selecting a saved set stages its filters

- **WHEN** a signed-in user selects a saved set from the **My filters** tab
- **THEN** the staged filters and the Show-results count update to that set, and the
  live job list is unchanged until **Show results** is activated

#### Scenario: Saving persists the staged filters

- **WHEN** a signed-in user saves the current edits as a new set from the **My filters**
  tab
- **THEN** the saved set captures the staged (currently-edited) filters

### Requirement: The filter modal and summary are reusable across catalogs

The two-pane modal chrome and the summary sidebar SHALL be factored into reusable
shells (a modal shell and a summary shell) driven by a rail definition, a deferred
(staged) facet store, and a pane renderer, so a non-job catalog can present the same
deferred two-pane "All filters" modal and chip-summary sidebar. The shell SHALL keep
the deferred-apply contract identical for every catalog: edits mutate a staged copy
and commit only on **Show results**.

#### Scenario: The companies catalog reuses the modal

- **WHEN** the companies catalog opens its **All filters** modal
- **THEN** it renders the same two-pane shell (rail + pane + deferred **Show results**
  footer) as the jobs modal, over the company facet set

#### Scenario: The staged contract holds for a reused modal

- **WHEN** a user stages a facet change in a reused modal and dismisses it without
  Show results
- **THEN** the underlying list is unchanged

### Requirement: The filter modal can apply the signed-in user's profile

The jobs filter modal SHALL offer, in its header, an **Apply my profile** action for a
signed-in user who has a saved profile. Activating it SHALL reset the staged filters and
then seed them from the user's profile: each profile specialization SHALL be staged as a
`category` value and each profile skill SHALL be staged as a `skills` value. When the
profile carries a `location_preferences` block, the action SHALL additionally seed the
location facets by flattening the three blocks: `work_mode` from `work_modes`; `regions`
from the union of `remote.regions` and `relocation.regions`; `countries` from the union of
`remote.countries`, `base.country`, and `relocation.countries`; `cities` from the union of
`base.city` and `relocation.cities`; and `relocation` staged as `supported` and `required`
when `relocation.open` is true. Empty or absent parts contribute nothing. The action
SHALL only stage — it SHALL NOT change the live job list; the seeded selection is applied
through the existing **Show results** commit, so the user previews (and MAY adjust) the
profile-derived filters before applying.

The action SHALL appear only on the full jobs filter modal (not on reuses that restrict
the rail to a facet subset, such as the profile-comparison modal). When the signed-in
user has no saved profile, the header SHALL instead present a link to create one at
`/my/profile`. When no user is signed in, neither the action nor the link SHALL appear.

#### Scenario: Applying the profile resets and seeds the staged filters

- **WHEN** a signed-in user with a saved profile (specializations `A`, `B`; skills `x`,
  `y`) has some unrelated staged filters and activates **Apply my profile**
- **THEN** the previously staged filters are cleared, the `category` facet is staged with
  `A` and `B`, the `skills` facet is staged with `x` and `y`, and the job list is
  unchanged until **Show results** is activated

#### Scenario: Applying a profile with location preferences seeds the location facets

- **WHEN** a signed-in user whose profile has `work_modes` `[remote, onsite]`,
  `remote.regions` `[latam]`, `base` `{country: br, city: "Florianópolis"}`, and
  `relocation` `{open: true, cities: ["Berlin"]}` activates **Apply my profile**
- **THEN** the staged filters include `work_mode` `[remote, onsite]`, `regions` `[latam]`,
  `countries` `[br]`, `cities` `["Florianópolis", "Berlin"]`, and `relocation`
  `[supported, required]`, and the job list is unchanged until **Show results** is activated

#### Scenario: Applying a profile without location preferences seeds no location facets

- **WHEN** a signed-in user whose profile has no `location_preferences` block activates
  **Apply my profile**
- **THEN** only the `category` and `skills` facets are staged and the location facets
  (`work_mode`, `regions`, `countries`, `cities`, `relocation`) remain empty

#### Scenario: Show results commits the profile-derived filters

- **WHEN** the user has applied their profile in the modal and then activates **Show
  results**
- **THEN** the staged selections become the live (URL-synced) filter state and the modal
  closes

#### Scenario: No profile shows a create-profile link instead

- **WHEN** a signed-in user who has no saved profile opens the jobs filter modal
- **THEN** the header shows a link to create a profile at `/my/profile` and no
  **Apply my profile** action

#### Scenario: Signed-out users see neither action nor link

- **WHEN** a signed-out user opens the jobs filter modal
- **THEN** the header shows neither the **Apply my profile** action nor the
  create-profile link

