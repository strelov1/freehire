## ADDED Requirements

### Requirement: Searchable jobs index

The system SHALL maintain a Meilisearch index of jobs with one document per job,
keyed by the job's internal `id`. Each document SHALL carry the fields needed to
both match and render a result without a follow-up database read: the searchable
text (title, company, description, location), the filterable facets, the
sortable fields, and the display fields returned to clients.

The index SHALL declare:

- **searchable attributes**: title, company, description, location.
- **filterable attributes**: source, remote, company_slug, work_mode,
  employment_type, seniority, category, domains, countries, company_type,
  company_size, visa_sponsorship, salary_currency, salary_period, skills,
  salary_min, salary_max, experience_years_min.
- **sortable attributes**: posted_at, salary_min, salary_max.

Facets derived from a job's `enrichment` JSONB SHALL be absent (or empty) on the
document when the job is not yet enriched; an unenriched job SHALL still be
indexed and findable by its text fields.

#### Scenario: A job is represented as one searchable document

- **WHEN** a job with title "Senior Go Developer", company "Acme", and a
  description is indexed
- **THEN** the `jobs` index holds one document keyed by that job's `id` whose
  searchable text includes the title, company, and description

#### Scenario: Unenriched job is still indexed

- **WHEN** a job with no enrichment is indexed
- **THEN** the document is present and matchable by its title/company/description
  text, with its enrichment-derived facets absent or empty

### Requirement: Hybrid keyword and semantic search

The `jobs` index SHALL be configured with an embedder whose model runs inside
Meilisearch (source `huggingFace`), requiring no external API key. Search
requests SHALL accept a semantic ratio that blends keyword and semantic ranking.
A ratio of 0 SHALL behave as pure keyword search; higher ratios SHALL weight
semantic similarity more. Keyword search SHALL remain fully functional
independent of the embedder.

#### Scenario: Pure keyword search

- **WHEN** a client searches with semantic ratio 0 for an exact term present in a
  job's text
- **THEN** the matching job is returned by keyword ranking

#### Scenario: Semantic blend returns related results

- **WHEN** a client searches with a non-zero semantic ratio for a query that is
  semantically related but not a literal substring of a job's text
- **THEN** semantically similar jobs are eligible to rank into the results

### Requirement: Public job search endpoint

The system SHALL expose `GET /api/v1/jobs/search` as a public (unauthenticated)
endpoint. It SHALL accept a free-text query `q`, facet filters matching the
index's filterable attributes, an optional sort, an optional semantic ratio, and
`limit`/`offset` pagination. The response SHALL use the standard list envelope
`{"data": [...], "meta": {...}}`, where `data` is the matched job documents and
`meta` carries at least the estimated total hit count and the applied
`limit`/`offset`. The existing `GET /api/v1/jobs` list endpoint SHALL be
unchanged.

Each result SHALL identify its job by `public_slug` and SHALL NOT include the
internal numeric `id`, consistent with the public-identity contract used by the
other public job reads.

#### Scenario: Keyword query returns matches

- **WHEN** a client requests `GET /api/v1/jobs/search?q=golang`
- **THEN** the response is `{"data": [...], "meta": {...}}` with jobs matching
  "golang" in `data` and the estimated total and pagination in `meta`

#### Scenario: Faceted filtering

- **WHEN** a client requests `GET /api/v1/jobs/search?q=engineer&seniority=senior&remote=true`
- **THEN** only jobs whose facets satisfy seniority=senior AND remote=true are
  returned

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

### Requirement: Batch reindex keeps the index in sync

The system SHALL provide a batch command that reads jobs from Postgres and
writes their documents to the Meilisearch `jobs` index in batches, suitable for
scheduled execution. The command SHALL ensure the index and its settings
(attributes, ranking rules, embedder) exist before indexing. Reindexing SHALL be
idempotent: running it again with unchanged data SHALL leave the index
representing the same set of jobs.

#### Scenario: Reindex populates the index

- **WHEN** the reindex command runs against a database containing jobs
- **THEN** the `jobs` index exists with the configured settings and contains one
  document per job

#### Scenario: Reindex is idempotent

- **WHEN** the reindex command runs twice with no change to the underlying jobs
- **THEN** the index represents the same set of job documents after the second
  run as after the first
