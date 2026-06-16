# web-frontend (delta)

## MODIFIED Requirements

### Requirement: Region (remote reach) filter facet

The frontend job-search filter UI SHALL offer a curated "Region" facet, rendered
as pills under the "Work format" facet, that filters on the search API's
`regions` parameter. Its options SHALL be the macro-region reach vocabulary
(Global, North America, LATAM, Europe, UK, MENA, Africa, APAC, CIS), each mapping
to a `regions` code (`global`, `north_america`, `latam`, `eu`, `uk`, `mena`,
`africa`, `apac`, `cis`) — one consistent macro level, with country-level
filtering handled by the separate Countries facet. The facet SHALL support
exclusion like the other facets. The facet's option values SHALL be codes from
the backend's `regions` vocabulary.

#### Scenario: Filtering by a region

- **WHEN** a user selects the "Europe" pill in the Region facet
- **THEN** the search request carries `regions=eu` and the results are jobs whose
  reach includes Europe

#### Scenario: Excluding a region

- **WHEN** a user excludes the "North America" pill
- **THEN** the search request excludes `regions=north_america` and such jobs are
  omitted

## ADDED Requirements

### Requirement: Open-vocabulary facet filters are distribution-driven selects

Facets with an open or high-cardinality vocabulary (Skills and Countries) SHALL
be filtered through a searchable select whose options come from the live facet
distribution (`GET /api/v1/jobs/facets`) rather than free-text entry, so the user
sees which values exist and how many open jobs each has under the current
filters. Each option SHALL display its job count, and options SHALL be ordered
by count (busiest first). Country options SHALL be labelled with a human-readable
country name derived from the ISO code. A value already selected but absent from
the current distribution SHALL remain listed so it stays removable.

The job-search view SHALL fetch the distribution under the same filter params as
the result list, debounced, and SHALL discard a stale (superseded) response so
the counts never reflect an older filter state.

#### Scenario: Skills/Countries options come from the distribution with counts

- **WHEN** the user opens the Skills or Countries filter section
- **THEN** the selectable options are the values present in the current facet
  distribution, each labelled with its job count and ordered busiest-first

#### Scenario: Country codes are shown as names

- **WHEN** a country option for ISO code `de` is rendered
- **THEN** its label reads `Germany`, not `de`

#### Scenario: A stale distribution response is ignored

- **WHEN** filters change rapidly and an earlier distribution request resolves
  after a later one
- **THEN** the later (current) response wins and the earlier one is discarded

### Requirement: Pill facets keep removed-vocabulary selections removable

A pill-control facet SHALL render any currently-selected value that has no
matching option as an active, removable pill, so a value removed from the
controlled vocabulary after a bookmark or saved search was created does not
become an invisible filter the user cannot clear from the UI.

#### Scenario: A removed region value stays removable

- **WHEN** the active filters include a region value no longer in the region
  vocabulary (e.g. an old `?regions=us` link after the macro-region change)
- **THEN** that value renders as an active pill the user can click to remove,
  rather than silently constraining results with no visible control
