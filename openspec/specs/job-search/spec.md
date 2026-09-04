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
- **sortable attributes**: posted_at, created_at, view_count,
  enrichment.salary_min, enrichment.salary_max. The endpoint's `sort` values are
  the bare names — a request says `sort=salary_min`, which the handler maps to the
  nested attribute. The two salary facets live under `enrichment`, the two dates
  and the view count do not, and this list names the ATTRIBUTES rather than the
  aliases. (`created_at` was already accepted and already declared in code; this
  list had drifted.)
- **one embedder**, named for the skill vector it carries, declared as
  `userProvided` at the width `skill-match-sort` assigns. Being `userProvided`
  means Meilisearch SHALL NOT call any model: the vectors are supplied by the
  indexers. Binary quantization SHALL be off — the vectors are sparse (a handful of
  non-zeros out of hundreds of dimensions), and quantizing them was measured to drop
  recall@20 from 95% to 10%.

The declared embedder width SHALL NOT change without a full index rebuild, and
until such a rebuild completes the index rejects documents carrying the new width.

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

A job with no skill vector SHALL still be indexed and fully searchable: the vector's
absence affects the match ordering alone, never that job's presence in the index or
its matching by text or any facet.

`view_count` requires no new document field and no new indexing path: the index
document embeds the public job projection, which already carries the counter, so
every document written since the counter existed already holds it. Declaring the
attribute sortable is the whole of the index-side change.

The counter's indexed value SHALL NOT be kept current through the search outbox.
The offline view-count worker (see `view-count-aggregation`) updates the column
daily without enqueueing, and the incremental push is gated on indexed content
changing, so the counter moves without triggering one. The scheduled full rebuild
reads the counter from Postgres on every run and runs more often than the daily
rollup that produces it, so the indexed figure is never staler than the source
figure. Outbox plumbing for this column would therefore add a queue path that
buys no freshness.

#### Scenario: The view count is sortable without a new document field

- **WHEN** the index settings declare `view_count` sortable
- **THEN** existing documents are orderable by it with no document rewrite,
  because the projection they embed already carries the counter

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

#### Scenario: The index declares a model-free embedder

- **WHEN** the index settings are applied
- **THEN** they declare exactly one embedder, sourced as `userProvided` and not
  binary-quantized, and applying them contacts no embedding service

#### Scenario: A job with no recognised skills is still fully searchable

- **WHEN** a job whose skills are all unrecognised is indexed
- **THEN** it holds no usable vector, and the job remains findable by text and
  filterable by every facet exactly as before

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

The `meta.total` for this endpoint SHALL be the exact open-job count from the
current catalogue-scale snapshot when one is available, and the approximate
estimate only when it is not. The endpoint SHALL NOT run a query whose cost
grows linearly with the catalogue size on each request — which is precisely why
the exact count is read from a precomputed snapshot rather than counted per
request.

Whichever figure is served, it SHALL describe the same set of postings the
endpoint paginates: open, not duplicate-suppressed, and not private. The
approximate fallback SHALL apply that full predicate, so it is an estimate of
the right set rather than an estimate of a larger one.

#### Scenario: List returns a page ordered newest-added first

- **WHEN** a client requests `GET /api/v1/jobs?limit=20&offset=0`
- **THEN** up to 20 open jobs are returned ordered by `created_at` descending
  (ties broken by `id` descending), in the `{"data": [...], "meta": {...}}`
  envelope

#### Scenario: Meta carries the exact total when a snapshot is available

- **WHEN** a catalogue-scale snapshot is available and a client requests
  `GET /api/v1/jobs?limit=20&offset=0`
- **THEN** `meta` reports the applied `limit` and `offset` and a `total` equal
  to the snapshot's exact open-job count

#### Scenario: Meta falls back to an approximate total when no snapshot exists

- **WHEN** no catalogue-scale snapshot is available and a client requests
  `GET /api/v1/jobs?limit=20&offset=0`
- **THEN** `meta` reports the applied `limit` and `offset` and a `total` that is
  an approximate open-job count, and the request succeeds

#### Scenario: The approximate estimate describes the paginated set

- **WHEN** the catalogue contains postings suppressed as duplicates or marked
  private and the approximate fallback is served
- **THEN** those postings are excluded from the estimate, as they are from the
  returned page

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
`sort` parameter SHALL always take precedence. `posted_at`, `created_at` and
`view_count` SHALL all be sortable attributes of the index and accepted `sort`
values. The DB-backed jobs list keeps its own stable default (`created_at`
descending) and is no longer required to match the search default — it accepts no
`sort` parameter at all, so every ordering the feed offers is served by the search
endpoint.

`sort=view_count` orders by how many people have opened each posting. Like the
other attribute sorts it honours `order` and defaults to descending, so the
common case — the most-viewed postings first — needs no second parameter.

`sort=match` is the one accepted value that is NOT an index attribute: it orders
by the caller's own skill vector (see `skill-match-sort`). When it is served, the
request SHALL carry no attribute sort directive at all, because an explicit sort
directive takes precedence over vector ranking in the engine and would silently
discard the ordering the caller asked for.

#### Scenario: Browsing without a query shows freshest postings first

- **WHEN** the search endpoint is called with empty `q` and no `sort`
- **THEN** results are ordered `posted_at` descending

#### Scenario: A text query keeps relevance order

- **WHEN** the search endpoint is called with `q=golang` and no `sort`
- **THEN** results are in relevance order (no sort directive)

#### Scenario: Explicit sort wins

- **WHEN** the search endpoint is called with `sort=created_at&order=desc`
- **THEN** results are ordered by `created_at` descending regardless of `q`

#### Scenario: Most-viewed ordering defaults to descending

- **WHEN** the search endpoint is called with `sort=view_count` and no `order`
- **THEN** results are ordered by `view_count` descending

#### Scenario: Most-viewed ordering honours an ascending order

- **WHEN** the search endpoint is called with `sort=view_count&order=asc`
- **THEN** results are ordered by `view_count` ascending

#### Scenario: A served match sort suppresses the attribute sort

- **WHEN** the search endpoint serves `sort=match` for an eligible caller
- **THEN** the engine request carries the caller's vector and no sort directive

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
requirement) remains responsible for reconciliation. Removing the documents of
closed jobs is no longer part of that reconciliation alone — it also has its own
incremental path (the "Closed jobs leave the facet index incrementally"
requirement), so the batch reindex bounds how stale the index can drift rather
than bounding how long a closed job stays searchable. A failure to push to the
index SHALL NOT fail ingest.

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

### Requirement: Closed jobs leave the facet index incrementally

The system SHALL remove a closed job's document from the live facet index without waiting
for the next batch reindex, so a vacancy that is no longer open stops matching search within
one drain cycle.

Removal SHALL be driven by a queue written in the same database transaction that closes the
job, so that a close and its pending removal cannot diverge. Because jobs are closed in bulk
— one statement closing every posting a crawl no longer saw — the enqueue SHALL be part of
the closing statement itself rather than a separate call per row.

A job removed from the catalogue outright SHALL also leave the index, on the same path. The
queue therefore SHALL NOT depend on the job's row still existing: a queued removal identifies
the document by key alone, and MUST survive the deletion of the job it refers to. A queue that
cascaded with the job would lose exactly the removals it exists to guarantee.

Removal SHALL be idempotent: removing a document that is not in the index is a no-op, so a
retried, overlapping, or duplicated removal is harmless and needs no coordination with the
indexing path.

A job that is queued both for indexing and for removal SHALL end up removed. The indexing
claim already excludes jobs that are not open, so a job closed after being queued for
indexing is never indexed by that entry.

Failure to remove SHALL NOT fail the run that closed the job, and SHALL leave the queue entry
for a later attempt rather than dropping it.

#### Scenario: A closed job stops matching search before the next batch reindex

- **WHEN** a crawl closes a job that was previously searchable
- **THEN** the job's document is removed from the live facet index and it no longer matches
  search, without waiting for a batch reindex

#### Scenario: A bulk close queues every job it closed

- **WHEN** one statement closes many jobs a crawl no longer saw
- **THEN** every closed job is queued for removal by that same statement

#### Scenario: A rolled-back close queues nothing

- **WHEN** the transaction that closes a job does not commit
- **THEN** no removal is queued for that job

#### Scenario: A hard-deleted job leaves the index

- **WHEN** a job is removed from the catalogue outright rather than closed
- **THEN** its removal is queued by the same statement that deleted it, and the document
  leaves the index

#### Scenario: A queued removal survives its job being deleted

- **WHEN** a job is queued for removal and its row is then hard-deleted before the queue is
  drained
- **THEN** the queued removal still exists and is still processed

#### Scenario: Removing an unindexed job is harmless

- **WHEN** a removal is processed for a job whose document is not in the index
- **THEN** the operation succeeds and the queue entry is completed

#### Scenario: A job queued for both indexing and removal ends up removed

- **WHEN** a job is queued for indexing and is then closed before that entry is drained
- **THEN** the job is not indexed by that entry, and its document is removed

#### Scenario: An unavailable search engine does not lose the removal

- **WHEN** the search engine is unavailable while a removal is being processed
- **THEN** the failure is logged, the queue entry remains for a later attempt, and the run
  that closed the job is unaffected

### Requirement: Experience years is filterable as a range

The search endpoints SHALL accept an `experience_years_max` parameter that bounds a
posting's stated experience requirement from above. When it is a non-negative
integer `N`, the search SHALL be restricted to documents whose
`enrichment.experience_years_min` is at most `N`. When the parameter is absent,
empty, negative, or not a valid integer, it SHALL impose no restriction. A negative
ceiling is rejected rather than honoured: the attribute is never below zero, so such
a filter can only match nothing, and serving an empty page would present a typo as a
legitimately narrow search.

The existing `experience_years_min` parameter keeps its current meaning unchanged —
it lower-bounds the same attribute (`enrichment.experience_years_min >= N`). The two
parameters SHALL compose as an AND, so supplying both expresses a closed range. The
addition is backward compatible: no existing parameter changes name or semantics.

Because `enrichment.experience_years_min` is absent on a posting whose experience
requirement was never stated, either bound SHALL exclude such postings. This
follows from Meilisearch's numeric comparison semantics and is deliberate — the
filter answers "the posting asks for at most N years", which an unstated
requirement does not satisfy.

#### Scenario: An upper bound restricts to postings asking for no more

- **WHEN** a client requests `GET /api/v1/jobs/search?experience_years_max=3`
- **THEN** only postings whose stated experience requirement is 3 years or fewer are
  returned

#### Scenario: Both bounds express a closed range

- **WHEN** a client requests
  `GET /api/v1/jobs/search?experience_years_min=2&experience_years_max=5`
- **THEN** only postings whose stated experience requirement is between 2 and 5
  years inclusive are returned

#### Scenario: An invalid upper bound imposes no restriction

- **WHEN** a client requests `GET /api/v1/jobs/search` with `experience_years_max`
  absent, empty, negative, or non-numeric
- **THEN** the result is not restricted by experience years

#### Scenario: Postings with no stated requirement fall outside a bounded range

- **WHEN** a posting carries no `enrichment.experience_years_min` and a client
  requests `experience_years_max=10`
- **THEN** that posting is not returned

#### Scenario: A zero upper bound selects the no-experience postings

- **WHEN** a client requests `GET /api/v1/jobs/search?experience_years_max=0`
- **THEN** only postings whose stated experience requirement is `0` are returned

### Requirement: The jobs feed can be ordered by profile match

The system SHALL accept `sort=match` on `GET /api/v1/jobs/search`, ordering
results by the cosine between each job's skill vector and the authenticated
caller's, computed from their profile skills.

The endpoint SHALL remain public. It SHALL attach the caller's identity when a
session is present but MUST NOT reject a request that lacks one.

Ranking SHALL compose with every existing facet filter in a single engine query,
and pagination SHALL behave as it does for any other ordering — there is no
window, and no ceiling beyond the endpoint's existing pagination guard.

The caller's vector SHALL be built per request from their profile alone. It SHALL
require no database read: the vector depends on nothing but the skills the profile
names.

**`sort=match` SHALL NEVER return an error.** Every reason it cannot be served —
no session, no profile, no skills, no recognised skills —
SHALL degrade to the endpoint's default ordering. A saved search or a shared link
carrying `sort=match` must not break when opened by someone it cannot be served
for, which is the same reason the jobs list ignores unknown filters rather than
refusing them.

#### Scenario: A signed-in caller with skills is ranked by match

- **WHEN** a caller with a session and a profile naming recognised skills requests
  `sort=match`
- **THEN** the engine query carries a vector of the declared width and results are
  ordered by their cosine against it

#### Scenario: An anonymous caller gets the default feed

- **WHEN** a caller with no session requests `sort=match`
- **THEN** the response is `200` with the default freshest-first ordering, and the
  engine query carries no vector

#### Scenario: A profile with no skills gets the default feed

- **WHEN** a signed-in caller whose profile names no skills requests `sort=match`
- **THEN** the response is `200` with the default ordering and no vector

#### Scenario: The match sort composes with facet filters

- **WHEN** an eligible caller requests `sort=match` together with country and
  seniority filters
- **THEN** one engine query carries both the vector and the facet filter, and
  neither is dropped

#### Scenario: Deep pagination under the match sort behaves normally

- **WHEN** an eligible caller pages into the match-ordered feed within the
  endpoint's pagination guard
- **THEN** results continue in match order, with no separate depth limit

