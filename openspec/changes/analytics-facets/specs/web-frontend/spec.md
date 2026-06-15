## ADDED Requirements

### Requirement: Analytics page

The web frontend SHALL provide a public `/analytics` page that visualizes the
facet-distribution counts from `GET /api/v1/jobs/facets`. The page SHALL render
each facet as a breakdown of values with their vacancy counts, sorted by count
descending, and SHALL display the total number of vacancies under the current
filters.

The page SHALL be server-side rendered for its initial state (counts under the
empty filter) so it is indexable and usable without client-side JavaScript for
the first paint.

#### Scenario: Initial render

- **WHEN** a visitor opens `/analytics`
- **THEN** the page shows the total vacancy count and per-facet breakdowns for
  the unfiltered catalogue, rendered server-side

#### Scenario: Breakdown ordering

- **WHEN** a facet breakdown is shown
- **THEN** its values are listed from highest count to lowest, each with its
  count and a proportional bar

### Requirement: Analytics drill-down

The analytics page SHALL let a visitor narrow the result set interactively by
selecting facet values, reusing the same URL-synced filter model as the jobs
browse page. Selecting a value SHALL update the URL and recompute every breakdown
to reflect the new filter set; the selection SHALL survive a page reload via the
URL.

#### Scenario: Selecting a facet value narrows the counts

- **WHEN** a visitor selects a value in a breakdown (e.g. `category = backend`)
- **THEN** the URL gains the corresponding filter param and all breakdowns and
  the total recompute to reflect the narrowed set

#### Scenario: Filters persist across reload

- **WHEN** a visitor reloads `/analytics` with filter params in the URL
- **THEN** the page renders the breakdowns and total for those filters
