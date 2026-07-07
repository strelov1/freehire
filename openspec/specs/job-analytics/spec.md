# job-analytics Specification

## Purpose
TBD - created by syncing change analytics-facets. Update Purpose after archive.
## Requirements
### Requirement: Facet distribution endpoint

The system SHALL expose a public, unauthenticated `GET /api/v1/jobs/facets`
endpoint that returns the count of open vacancies per facet value under a given
set of filters, instead of a page of jobs. The endpoint SHALL accept the same
query parameters as `GET /api/v1/jobs/search` (the full-text `q` and every facet
filter param, including repeated values, `<param>_mode`, `<param>_exclude`, and
numeric range filters) and apply them with identical semantics.

The response SHALL use the single-item envelope `{"data": ...}` whose `data`
object contains:
- `total`: the estimated number of vacancies matching the filters,
- `facets`: a map of facet → (value → count),
- `stats`: a map of numeric facet → `{min, max}`.

Both maps SHALL be keyed by the **public facet param name** (the same name the
client filters with, e.g. `seniority`, `salary_min`) — never the internal index
attribute (`enrichment.seniority`). Continuous numeric facets (salary,
experience) SHALL appear only under `stats`, not as a per-value distribution
under `facets`.

#### Scenario: Counts under no filter

- **WHEN** a client requests `GET /api/v1/jobs/facets` with no query params
- **THEN** the response is `200` with `data.total` set and `data.facets`
  containing a value→count map for every facetable attribute

#### Scenario: Counts under an active filter

- **WHEN** a client requests `GET /api/v1/jobs/facets?regions=eu&category=backend`
- **THEN** the returned counts reflect only vacancies matching `regions=eu` AND
  `category=backend`, with the same filter semantics as `/jobs/search`

#### Scenario: Numeric stats

- **WHEN** the response is computed
- **THEN** `data.stats` reports the min and max for numeric facets under their
  public param name (e.g. `salary_min`) over the filtered set, and those facets
  do not appear under `data.facets`

#### Scenario: Search not configured

- **WHEN** the search backend is not configured (no Meilisearch)
- **THEN** the endpoint responds `503` with an error envelope, matching the
  behaviour of `/jobs/search`

### Requirement: High-cardinality facets are not truncated

The search index SHALL be configured so that facet distributions return enough
values that high-cardinality facets (such as skills and countries) are not capped
at the engine default.

#### Scenario: Skills distribution exceeds the default cap

- **WHEN** more than 100 distinct skill values exist across the filtered set
- **THEN** the facet distribution for `skills` returns more than 100 values
  (subject to the configured `maxValuesPerFacet`)

### Requirement: Disjunctive facet distribution

The facet-distribution endpoint SHALL support a **disjunctive** mode (opt-in via a
`disjunctive` query flag). In this mode, each requested facet's distribution SHALL
be computed under the full filter **with that facet's own selection removed**
(its `<param>`, `<param>_exclude`, and `<param>_mode` values excluded), so a
facet's own selection does not zero out its sibling values. The reported `total`
SHALL still be the estimated count under the **full** filter (all facets applied)
— the number the "Show N results" action reflects. Non-disjunctive requests keep
the existing conjunctive behaviour (every facet counted under the full filter).

The disjunctive distributions SHALL be produced by running the per-facet queries
**concurrently** (a `search` capability), so the endpoint's latency is that of a
single facet query rather than the sum of all of them.

#### Scenario: A facet's own selection does not hide its siblings

- **WHEN** a client requests the disjunctive facet distribution with
  `seniority=senior` selected
- **THEN** the `seniority` distribution still reports counts for the other
  seniorities (e.g. `junior`, `middle`) — each counted under the rest of the
  filter, ignoring the `senior` selection
- **AND** a different facet (e.g. `category`) is counted under a filter that
  **does** include `seniority=senior`

#### Scenario: The total reflects the full filter

- **WHEN** a client requests the disjunctive distribution with several facets
  selected
- **THEN** the response `total` is the estimated job count under all selected
  facets combined (not any single facet's disjunctive subtotal)

#### Scenario: Non-disjunctive requests are unchanged

- **WHEN** a client requests the facet distribution without the disjunctive flag
- **THEN** every facet is counted under the full filter (conjunctive), exactly as
  before

