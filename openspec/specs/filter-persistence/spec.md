# filter-persistence Specification

## Purpose
TBD - created by archiving change persist-job-filters-localstorage. Update Purpose after archive.
## Requirements
### Requirement: Persist the standalone job filter set on explicit change
The system SHALL persist the standalone `/jobs` filter set to browser storage (key `hire.jobFilters`, as the serialized filter query string) whenever the user explicitly changes it, and SHALL NOT write storage in response to navigation-driven re-seeding (back/forward or landing on a URL).

#### Scenario: User adjusts a filter
- **WHEN** a signed-in or anonymous user toggles a facet, edits the search text, moves a slider, or applies a saved search on the standalone `/jobs` list
- **THEN** the serialized filter query string is written to `hire.jobFilters`

#### Scenario: User clears all filters
- **WHEN** the user resets the filters (Clear all) so the applied filter set is empty
- **THEN** the `hire.jobFilters` key is removed, so no filters are restored later

#### Scenario: Navigation does not overwrite storage
- **WHEN** the filter state is re-seeded by a browser back/forward or an incoming navigation rather than a user edit
- **THEN** `hire.jobFilters` is left unchanged

### Requirement: Restore stored filters when returning to a bare /jobs
The system SHALL restore the stored filter set when a client-side navigation lands on the standalone `/jobs` with no filter params in the URL and a non-empty value exists in storage; it SHALL rewrite the URL to reflect the restored filters and reload the list. A cold first load of the page (a hard load, refresh, or direct URL — the router's initial `enter`) SHALL NOT restore and SHALL NOT error; the URL as loaded is served.

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
- **THEN** the unfiltered list is shown and nothing is restored

### Requirement: URL filter params take precedence over storage
The system SHALL apply the URL's filter params when they are present on `/jobs`, ignoring any stored value. A pure link/refresh load SHALL leave storage untouched (storage mirrors only explicit user changes); once the user interacts with the URL-seeded filters, that explicit change updates storage as usual.

#### Scenario: Opening a shared filtered link
- **WHEN** the user opens `/jobs?regions=US` while `hire.jobFilters` holds a different set
- **THEN** the URL's filters (`regions=US`) are applied and displayed, and `hire.jobFilters` is left unchanged until the user next edits a filter

### Requirement: Persistence is limited to the standalone jobs list
The system SHALL NOT persist or restore filters for the company-embedded jobs list, so the shared storage key reflects only the standalone `/jobs` filters.

#### Scenario: Filtering a company's postings
- **WHEN** the user filters the embedded jobs list on `/companies/:slug`
- **THEN** `hire.jobFilters` is neither written nor read for that list

### Requirement: Storage failures must not break the page
The system SHALL degrade gracefully when browser storage is unavailable or throws, keeping the URL as the working source of filter state.

#### Scenario: Storage access throws
- **WHEN** reading or writing `localStorage` fails (private mode, quota, or disabled storage)
- **THEN** filtering continues to work from the URL and no error is surfaced to the user

