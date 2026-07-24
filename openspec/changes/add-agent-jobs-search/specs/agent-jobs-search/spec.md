## ADDED Requirements

### Requirement: Agent job search endpoint

The system SHALL expose `GET /api/v1/agent/jobs/search` as a public
(unauthenticated) endpoint that runs the same job search as `GET /api/v1/jobs/search`:
it SHALL accept the same free-text query `q`, the same facet filters, sort,
semantic ratio, and `limit`/`offset` pagination, and SHALL apply the same
pagination-window guard. Ranking, faceting, pagination, and the estimated total
SHALL be produced by the search index exactly as the public search endpoint
produces them. The response SHALL use the standard list envelope
`{"data": [...], "meta": {...}}`, where `data` is the matched jobs and `meta`
carries at least the estimated total hit count and the applied `limit`/`offset`.
Each result SHALL identify its job by `public_slug` and SHALL NOT include the
internal numeric `id`.

Unlike the public search endpoint, each result's `description` SHALL be the job's
**full** description, read verbatim from Postgres and keyed by the result's
internal id — never the truncated index preview. The hydration SHALL be
best-effort per result: a returned hit whose id has no corresponding Postgres row
(e.g. the index lags a just-removed job) SHALL retain whatever description the
index served and SHALL NOT be dropped from the response.

#### Scenario: Runs the same search as the public endpoint

- **WHEN** a client requests
  `GET /api/v1/agent/jobs/search?q=engineer&seniority=senior&regions=eu&limit=10`
- **THEN** the matched, ranked, paginated results and the `meta` totals are those
  the search index produces for the same query and filters, in the standard
  `{"data": [...], "meta": {...}}` envelope

#### Scenario: Results carry the full description, not the preview

- **WHEN** a job whose description exceeds the index preview cap is returned by
  `GET /api/v1/agent/jobs/search`
- **THEN** the result's `description` equals the full description that
  `GET /api/v1/jobs/{slug}` returns for the same job, not the truncated preview

#### Scenario: Results identify jobs by public slug, not internal id

- **WHEN** a job is returned by `GET /api/v1/agent/jobs/search`
- **THEN** the result carries the job's `public_slug` and omits the internal
  numeric `id`

#### Scenario: Hydration is best-effort on a stale hit

- **WHEN** a returned hit's id has no matching Postgres row (the index lags a
  just-removed job)
- **THEN** that hit is still present in the response rather than being dropped

### Requirement: Selectable description format

The endpoint SHALL accept a `description_format` parameter selecting how each
result's full description is represented:

- `html` (the default when the parameter is absent, empty, or unrecognized): the
  stored verbatim HTML.
- `text`: the description with HTML tags removed, as plain text.
- `markdown`: the description with its HTML converted to Markdown, preserving
  block structure such as lists and headings.

The selected format SHALL apply uniformly to every result in the response. The
format SHALL affect only the `description` field; all other fields SHALL be
unchanged.

#### Scenario: Default format is verbatim HTML

- **WHEN** a client requests `GET /api/v1/agent/jobs/search?q=go` without
  `description_format`
- **THEN** each result's `description` is the stored verbatim HTML

#### Scenario: text format strips HTML

- **WHEN** a client requests `GET /api/v1/agent/jobs/search?q=go&description_format=text`
- **THEN** each result's `description` contains the description's text with HTML
  tags removed

#### Scenario: markdown format converts HTML to Markdown

- **WHEN** a client requests `GET /api/v1/agent/jobs/search?q=go&description_format=markdown`
- **THEN** each result's `description` is the description rendered as Markdown,
  with block structure such as lists and headings preserved

#### Scenario: Unrecognized format falls back to the default

- **WHEN** a client requests `GET /api/v1/agent/jobs/search?description_format=xml`
- **THEN** the response is served with the default `html` representation rather
  than failing
