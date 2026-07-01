## MODIFIED Requirements

### Requirement: Public job search endpoint

The system SHALL expose `GET /api/v1/jobs/search` as a public (unauthenticated)
endpoint. It SHALL accept a free-text query `q`, facet filters matching the
index's filterable attributes, an optional sort, an optional semantic ratio, and
`limit`/`offset` pagination. Facet filters SHALL include `regions` (the geography
facet) and SHALL NOT include the removed raw `remote` filter. The response SHALL
use the standard list envelope `{"data": [...], "meta": {...}}`, where `data` is
the matched job documents and `meta` carries at least the estimated total hit
count and the applied `limit`/`offset`. The separate DB-backed `GET /api/v1/jobs`
list endpoint is governed by its own requirement (see "DB-backed jobs list is
index-served with an approximate total").

The endpoint SHALL additionally accept a `posted_within_days` parameter. When it
is a positive integer `N`, the search SHALL be restricted to jobs whose
`posted_ts` is at or after `now - N*86400` (i.e. posted within the last `N`
days), where `now` is the time the request is served. When the parameter is
absent, empty, zero, negative, or not a valid integer, it SHALL impose no date
restriction. The filter SHALL compose with the other facet filters (AND).

Each result SHALL identify its job by `public_slug` and SHALL NOT include the
internal numeric `id`, consistent with the public-identity contract used by the
other public job reads.

#### Scenario: Keyword query returns matches

- **WHEN** a client requests `GET /api/v1/jobs/search?q=golang`
- **THEN** the response is `{"data": [...], "meta": {...}}` with jobs matching
  "golang" in `data` and the estimated total and pagination in `meta`

#### Scenario: Faceted filtering by region

- **WHEN** a client requests
  `GET /api/v1/jobs/search?q=engineer&seniority=senior&regions=eu`
- **THEN** only jobs whose facets satisfy seniority=senior AND whose top-level
  `regions` include `eu` are returned

#### Scenario: Empty query browses with filters

- **WHEN** a client requests `GET /api/v1/jobs/search` with filters but no `q`
- **THEN** the filtered jobs are returned ranked by the index defaults

#### Scenario: Pagination is reflected in meta

- **WHEN** a client requests `GET /api/v1/jobs/search?q=go&limit=10&offset=20`
- **THEN** at most 10 documents are returned and `meta` reports the applied
  `limit` 10 and `offset` 20 alongside the estimated total

#### Scenario: Results identify jobs by public slug, not internal id

- **WHEN** a job is returned by `GET /api/v1/jobs/search`
- **THEN** the result carries the job's `public_slug` and omits the internal
  numeric `id`

#### Scenario: Freshness filter restricts to recent postings

- **WHEN** a client requests `GET /api/v1/jobs/search?posted_within_days=7`
- **THEN** only jobs whose effective posting date is within the last 7 days are
  returned

#### Scenario: Invalid freshness value imposes no restriction

- **WHEN** a client requests `GET /api/v1/jobs/search` with `posted_within_days`
  absent, zero, negative, or non-numeric
- **THEN** the result is not restricted by posting date

## ADDED Requirements

### Requirement: DB-backed jobs list is index-served with an approximate total

The DB-backed `GET /api/v1/jobs` list endpoint SHALL return open jobs
(`closed_at IS NULL`) ordered newest-added first (`created_at` descending, `id`
descending) with `limit`/`offset` pagination, using the standard list envelope
`{"data": [...], "meta": {...}}`. The ordered page SHALL be served through a
partial index matching that order (no full-table sort at request time), so the
endpoint stays responsive at catalogue scale (millions of open jobs).

The `meta.total` for this endpoint SHALL be an **approximate** estimate of the
open-job count, not an exact `count(*)` over the whole open set — mirroring how
`/jobs/search` already reports an *estimated* total. The endpoint SHALL NOT run a
query whose cost grows linearly with the catalogue size on each request.

#### Scenario: List returns a page ordered newest-added first

- **WHEN** a client requests `GET /api/v1/jobs?limit=20&offset=0`
- **THEN** up to 20 open jobs are returned ordered by `created_at` descending
  (ties broken by `id` descending), in the `{"data": [...], "meta": {...}}`
  envelope

#### Scenario: Meta carries an approximate total and the applied pagination

- **WHEN** a client requests `GET /api/v1/jobs?limit=20&offset=0`
- **THEN** `meta` reports the applied `limit` and `offset` and a `total` that is
  an approximate open-job count (not required to equal an exact `count(*)`)
