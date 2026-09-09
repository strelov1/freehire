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

`q` matches against four fields: `title`, `company`, `description`, and
`location`. An unquoted multi-word `q` SHALL match as an OR of its (stemmed)
tokens: a document matching any token across those fields is a hit. A
double-quoted `q` (e.g. `q="systems engineer"`) SHALL match as an AND of its
(stemmed) tokens, irrespective of the tokens' order or adjacency in the matched
field — quoting SHALL NOT be interpreted as a contiguous-phrase match.

The endpoint SHALL additionally accept a `q_fields` parameter restricting which
of the four `q`-searchable fields (`title`, `company`, `description`, `location`)
`q` is matched against. `q_fields` SHALL accept one or more of those field names,
either as a repeated query parameter or as a single comma-separated value (the
two forms SHALL resolve identically). When absent, `q` matches across all four
fields as described above. If `q_fields` names ANY field outside that set of
four — whether alone or alongside otherwise-valid names — the ENTIRE parameter
SHALL be dropped: `q` SHALL match unrestricted across all four fields, exactly
as if `q_fields` were absent, and `q_fields` SHALL be reported via
`meta.ignored_params`, consistent with how every other unrecognized search
parameter is handled. Recognized names SHALL NOT be partially applied when the
value also contains an unrecognized one.

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

#### Scenario: Unquoted multi-word query is OR-of-tokens

- **WHEN** a client requests `GET /api/v1/jobs/search?q=systems%20engineer`
- **THEN** the results include jobs matching either "systems" or "engineer" (or
  both) in any of `title`, `company`, `description`, or `location`

#### Scenario: Quoted query is order-independent AND-of-tokens, not a phrase match

- **WHEN** a client requests `GET /api/v1/jobs/search?q=%22engineer%20systems%22`
  (quoted, reversed word order)
- **THEN** the results are identical to `q=%22systems%20engineer%22`, and MAY
  include a job whose matched field contains both tokens non-adjacently or in a
  different attribute than the title (e.g. "systems" only in `company`)

#### Scenario: q_fields restricts matching to the named field

- **WHEN** a client requests `GET /api/v1/jobs/search?q=systems&q_fields=title`
- **THEN** only jobs whose `title` matches "systems" are returned, regardless of
  whether "systems" also appears in that job's `company`, `description`, or
  `location`

#### Scenario: A repeated q_fields key is the same as a comma-joined value

- **WHEN** a client requests
  `GET /api/v1/jobs/search?q=systems&q_fields=title&q_fields=company`
- **THEN** the results are identical to
  `GET /api/v1/jobs/search?q=systems&q_fields=title,company`

#### Scenario: Unrecognized q_fields value is reported, not applied

- **WHEN** a client requests `GET /api/v1/jobs/search?q=systems&q_fields=salary`
- **THEN** the response is unrestricted by `q_fields` (as if it were absent) and
  `meta.ignored_params` includes `q_fields`

#### Scenario: One unrecognized name in a mixed q_fields value drops it entirely

- **WHEN** a client requests
  `GET /api/v1/jobs/search?q=systems&q_fields=title,salary`
- **THEN** the response is unrestricted by `q_fields` (as if it were absent) —
  `title` is NOT applied on its own — and `meta.ignored_params` includes
  `q_fields`

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
