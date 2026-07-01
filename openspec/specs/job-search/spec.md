# job-search Specification

## Purpose
TBD - created by archiving change add-job-search. Update Purpose after archive.
## Requirements
### Requirement: Searchable jobs index

The system SHALL maintain a Meilisearch index of jobs with one document per job,
keyed by the job's internal `id`. Each document SHALL carry the fields needed to
both match and render a result without a follow-up database read: the searchable
text (title, company, description, location), the filterable facets, the
sortable fields, and the display fields returned to clients.

The index SHALL declare:
- **searchable attributes**: title, company, description, location.
- **filterable attributes**: source, company_slug, work_mode, employment_type,
  seniority, category, domains, regions, countries, company_type, company_size,
  visa_sponsorship, salary_currency, salary_period, skills, salary_min,
  salary_max, experience_years_min, and `posted_ts`. The raw `remote` flag SHALL
  NOT be a filterable attribute (work_mode subsumes it).
- **sortable attributes**: posted_at, salary_min, salary_max.

Each document SHALL carry a derived numeric `posted_ts` field: the unix-seconds
value of the job's **effective** posting date — the source's `posted_at` when
present and not in the future, otherwise the ingest time (`created_at`) — the
same value, in epoch form, that the document's display `posted_at` reflects.
`posted_ts` is an index-only field: it SHALL be filterable but SHALL NOT appear
in the public job wire shape returned by the job read endpoints. Because
`posted_ts` is derived at index time, no Postgres column or backfill is
required; a reindex SHALL populate it on existing jobs.

Geography and work mode are filtered through the document's **top-level**
`regions`, `countries`, and `work_mode` fields — the resolved union/precedence of
the location-derived columns and the enrichment-derived values — not through the
`enrichment.*` dot paths. There SHALL be no separate
`enrichment.regions`/`enrichment.countries`/`enrichment.work_mode` facet on the
document.

Facets derived from a job's `enrichment` JSONB SHALL be absent (or empty) on the
document when the job is not yet enriched; an unenriched job SHALL still be
indexed and findable by its text fields, and SHALL still carry any geography
parsed from its location.

#### Scenario: A job is represented as one searchable document

- **WHEN** a job with title "Senior Go Developer", company "Acme", and a
  description is indexed
- **THEN** the `jobs` index holds one document keyed by that job's `id` whose
  searchable text includes the title, company, and description

#### Scenario: Unenriched job is still indexed with its parsed geography

- **WHEN** a job with no enrichment but location `Remote - USA` is indexed
- **THEN** the document is present and matchable by its text, with its
  enrichment-derived facets absent or empty and its top-level `regions`/
  `countries` carrying the parsed geography

#### Scenario: Geography is filterable via the top-level regions facet

- **WHEN** a job whose unioned geography includes `eu` is indexed
- **THEN** it is returned by a filter on `regions = "eu"`

#### Scenario: Document carries the effective posting date as an epoch

- **WHEN** a job whose effective posting date is a given instant is indexed
- **THEN** its document carries `posted_ts` equal to that instant in unix
  seconds, and a job with a null or future `posted_at` carries the `created_at`
  instant instead — matching its display `posted_at`

#### Scenario: posted_ts is filterable but not in the public job shape

- **WHEN** a job document is indexed and the same job is read through a public
  job endpoint
- **THEN** the document is filterable by a `posted_ts` numeric range, while the
  public job wire shape does not include a `posted_ts` field

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

### Requirement: Batch reindex keeps the index in sync

The system SHALL provide a batch command that reads jobs from Postgres and
writes their documents to the Meilisearch `jobs` index in batches, suitable for
scheduled execution. The command SHALL ensure the index and its settings
(attributes, ranking rules, embedder) exist before indexing. Reindexing SHALL be
idempotent: running it again with unchanged data SHALL leave the index
representing the same set of jobs.

The index SHALL contain documents only for **open** jobs: the reindex command
SHALL index open jobs and SHALL remove the documents of jobs that have been
closed (`closed_at` set) since the previous run. A reopened job SHALL be indexed
again on the next run.

#### Scenario: Reindex populates the index

- **WHEN** the reindex command runs against a database containing jobs
- **THEN** the `jobs` index exists with the configured settings and contains one
  document per open job

#### Scenario: Reindex is idempotent

- **WHEN** the reindex command runs twice with no change to the underlying jobs
- **THEN** the index represents the same set of job documents after the second
  run as after the first

#### Scenario: Closed job is dropped on reindex

- **WHEN** a job is closed and a reindex runs
- **THEN** the job's document is removed from the index and no longer matches any
  search

#### Scenario: Reopened job returns to the index

- **WHEN** a previously closed job is reopened and a reindex runs
- **THEN** the job's document is indexed again

### Requirement: Default ordering is newest-added first

A search request with no query text and no valid `sort` parameter SHALL return
jobs ordered by the source's posting date (`posted_at`), newest first. A request
with query text and no `sort` SHALL keep relevance order. An explicit valid
`sort` parameter SHALL always take precedence. Both `posted_at` and `created_at`
SHALL be sortable attributes of the index and accepted `sort` values. The
DB-backed jobs list keeps its own stable default (`created_at` descending) and is
no longer required to match the search default.

#### Scenario: Browsing without a query shows freshest postings first

- **WHEN** the search endpoint is called with empty `q` and no `sort`
- **THEN** results are ordered `posted_at` descending

#### Scenario: A text query keeps relevance order

- **WHEN** the search endpoint is called with `q=golang` and no `sort`
- **THEN** results are in relevance order (no sort directive)

#### Scenario: Explicit sort wins

- **WHEN** the search endpoint is called with `sort=created_at&order=desc`
- **THEN** results are ordered by `created_at` descending regardless of `q`

### Requirement: Incremental indexing keeps new and changed jobs fresh

The system SHALL index a job into the live Meilisearch facet index as soon as
ingest persists it with new or changed indexed content, so a newly ingested or
edited open job becomes searchable within one crawl cycle rather than only after
the next scheduled batch reindex. A job whose indexed content did not change on a
re-ingest (for example, an upsert that only refreshes its last-seen timestamp)
SHALL NOT be re-pushed. This incremental path SHALL target the facet/keyword
production index only; the semantic index keeps its separate schedule.

Incremental indexing SHALL be best-effort and SHALL NOT change the source of
truth: the batch reindex (the "Batch reindex keeps the index in sync"
requirement) remains responsible for reconciliation, including removing the
documents of closed jobs. A failure to push to the index SHALL NOT fail ingest.

#### Scenario: A newly ingested job is searchable before the next batch reindex

- **WHEN** ingest persists a job that was not previously in the catalogue
- **THEN** the job's document is present in the live facet index and the job
  matches search without waiting for a batch reindex

#### Scenario: An edited job is re-indexed on re-ingest

- **WHEN** a job already in the catalogue is re-ingested with an edited title or
  description
- **THEN** the job's document in the live facet index reflects the edit without
  waiting for a batch reindex

#### Scenario: An unchanged re-ingest does not re-push the document

- **WHEN** a job already in the catalogue is re-ingested with no change to its
  indexed content
- **THEN** no document push is issued for that job

#### Scenario: An index failure does not fail ingest

- **WHEN** the search engine is unavailable while ingest is pushing new documents
- **THEN** the ingest run records the persisted jobs and completes, and the
  failure is logged rather than aborting the run

