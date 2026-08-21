## MODIFIED Requirements

### Requirement: Restore stored filters when returning to a bare /jobs
The system SHALL restore the stored filter set when a client-side navigation lands on the standalone `/jobs` with no filter params in the URL and a non-empty value exists in storage; it SHALL rewrite the URL to reflect the restored filters and reload the list. A cold first load of the page (a hard load, refresh, or direct URL — the router's initial `enter`) SHALL NOT restore and SHALL NOT error; the URL as loaded is served.

A stored set that is restored SHALL suppress the IP-derived opening scope (see the
`geo-default-scope` capability), whether or not the stored set names any geography.
The derived scope is not a restore and is not subject to the cold-load rule above: it
is the fallback for a browser that has never stored a filter set, and it therefore
does apply on a cold load. It is never written to `hire.jobFilters` — storage records
what the visitor chose, and a guess is not a choice.

#### Scenario: Clicking the Jobs nav from another page
- **WHEN** the user has stored filters and navigates to a bare `/jobs` (e.g. the "Jobs" nav link) from another route
- **THEN** the stored filters are applied, the URL is rewritten to reflect them, and the list reloads filtered

#### Scenario: Clicking Jobs while already on a filtered /jobs
- **WHEN** the user is on `/jobs?…` with active filters and triggers a navigation to a bare `/jobs`
- **THEN** the stored filters are restored rather than the list being cleared

#### Scenario: Cold load of a bare /jobs
- **WHEN** the user hard-loads or directly opens a bare `/jobs` (the initial `enter`) while `hire.jobFilters` holds a value
- **THEN** the unfiltered list is served without a restore and without any error, and the stored value is preserved for a later client-side return

#### Scenario: No stored filters
- **WHEN** the user lands on a bare `/jobs` and `hire.jobFilters` is absent or empty
- **THEN** nothing is restored; the list is shown unfiltered unless the IP-derived opening scope applies, and no restore is recorded either way

#### Scenario: A restored set suppresses the derived scope
- **WHEN** a stored filter set is restored on a bare `/jobs` and the visitor's country would have derived a region
- **THEN** the restored set is applied alone and the derived region is not added to it

## ADDED Requirements

### Requirement: The derived-scope marker is stored apart from the filter set
The system SHALL keep the record of whether the IP-derived opening scope has been
offered in a browser-storage key distinct from `hire.jobFilters`, and SHALL NOT
remove that record when the filter set is cleared.

Clearing the filters removes `hire.jobFilters` entirely. A derived scope that keyed
on "storage is empty" would therefore re-apply on the next visit and undo the clear,
every time, which is the one failure mode this separation exists to prevent.

#### Scenario: Clearing filters leaves the marker
- **WHEN** the user clears all filters, so `hire.jobFilters` is removed
- **THEN** the derived-scope marker is left in place and the guess is not offered again

#### Scenario: The marker alone does not restore anything
- **WHEN** the derived-scope marker is present and `hire.jobFilters` is absent
- **THEN** a bare `/jobs` shows the unfiltered list
