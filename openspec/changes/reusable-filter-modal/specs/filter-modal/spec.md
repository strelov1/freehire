## MODIFIED Requirements

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

## ADDED Requirements

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
