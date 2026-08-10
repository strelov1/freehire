## ADDED Requirements

### Requirement: List-valued filter params accept comma-separated values

For every list-valued job-filter facet param (every entry in the `StringFacets`
set, e.g. `skills`, `regions`, `category`, and their `<param>_exclude`
counterparts), the system SHALL accept a single query-string entry whose value
is a comma-separated list, resolving it to the same set of selected values as
if each item had been sent as its own repeated key. This applies wherever job
filters are read from a query string: the public `/jobs`-family API endpoints
and the web app's own filter-bar URL state.

#### Scenario: Comma-separated values resolve to multiple selections

- **WHEN** a request includes `skills=go,react`
- **THEN** the `skills` facet filters on both `go` and `react`, the same as
  `skills=go&skills=react` would

#### Scenario: Comma-separated values apply to exclude params too

- **WHEN** a request includes `skills_exclude=java,cpp`
- **THEN** jobs tagged `java` or `cpp` are excluded, the same as
  `skills_exclude=java&skills_exclude=cpp` would

### Requirement: Repeated-key values remain accepted

The system SHALL continue to accept the existing repeated-key form
(`skills=go&skills=react`) for every list-valued facet param, with unchanged
behavior, so that URLs built before this change — bookmarks, saved
`search_profiles`, subscription alert URLs, and third-party clients — continue
to work with no migration.

#### Scenario: Existing repeated-key URLs behave unchanged

- **WHEN** a request includes `skills=go&skills=react` (no commas)
- **THEN** the `skills` facet filters on both `go` and `react`, exactly as
  before this change

#### Scenario: Repeated keys and comma-separated values can mix

- **WHEN** a request includes `skills=go,react&skills=aws`
- **THEN** the `skills` facet filters on `go`, `react`, and `aws` — the union
  of every value found across all occurrences of the param

### Requirement: Web app filter bar serializes selections compactly

The web app's job-filter URL state SHALL serialize each facet's selected
values as one comma-joined query-string entry rather than one query pair per
value, for both the include and `_exclude` params.

#### Scenario: Multiple selected skills produce one query entry

- **WHEN** a user selects the skills "go" and "react" in the filter UI
- **THEN** the resulting URL contains one `skills=go,react` entry rather than
  two `skills=` entries

#### Scenario: No selection omits the param

- **WHEN** a user has no values selected for a facet
- **THEN** that facet's param is absent from the URL, unchanged from prior
  behavior
