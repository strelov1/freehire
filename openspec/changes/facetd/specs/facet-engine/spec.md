## ADDED Requirements

### Requirement: facetd indexes the documents the jobs index holds

facetd SHALL index the same `JobDocument` values the Meilisearch `jobs` index receives, flattened
the way Meilisearch reads attributes: nested objects by dot path, each array element as a value,
booleans as `true`/`false`, numbers both as a value and in a sorted numeric column, an empty array
or a `null` as present-but-empty. Vectors (`_vectors`) and free-text fields SHALL NOT be indexed.

#### Scenario: A nested enrichment attribute is filterable by its dot path

- **WHEN** a document carrying `enrichment.seniority = "senior"` is indexed
- **THEN** the fragment `enrichment.seniority = "senior"` matches it

#### Scenario: An array attribute matches on any element

- **WHEN** a document with `skills = ["go", "python"]` is indexed
- **THEN** both `skills = "go"` and `skills = "python"` match it

### Requirement: facetd hides closed and duplicate documents by itself

facetd SHALL exclude every document marked `is_closed` or `is_duplicate`, and every deleted
document, from every answer, without the caller adding a fragment for it.

#### Scenario: A closed job leaves the counts at once

- **WHEN** `CloseJobs([42])` is applied to facetd
- **THEN** the next facet count no longer includes job 42

### Requirement: facetd evaluates our filter shapes exactly as Meilisearch v1.49 does

facetd SHALL accept a filter as `nil`, a string, `[]string` or `[][]string` (outer AND, inner OR),
whose fragments are the shapes `internal/search/search` builds: `=`, `!=`, `>=`, `<=`, `>`, `<`,
`IN [..]`, `NOT IN [..]`, `EXISTS`, `NOT EXISTS`, `IS EMPTY`, `IS NOT EMPTY`, and `NOT` before any
of them. It SHALL return the same document set Meilisearch v1.49 returns for the same documents.

#### Scenario: Not-equal matches documents without the attribute

- **WHEN** some documents carry `enrichment.category` and others do not, and the filter is `enrichment.category != "sales"`
- **THEN** the result holds every document whose category is not `sales`, including those without the attribute

#### Scenario: Not-equal on an attribute no document carries matches nothing

- **WHEN** no indexed document carries `enrichment.company_size` and the filter is `enrichment.company_size != "large"`
- **THEN** the result is empty

#### Scenario: A shape outside the supported grammar is refused

- **WHEN** a fragment does not parse as one of the supported shapes
- **THEN** facetd returns an "unsupported filter" error and does not treat the fragment as match-all

### Requirement: facetd answers only requests without free text

facetd SHALL answer facet distributions, totals and distinct-value counts for requests whose query
text is empty, and SHALL refuse a request carrying query text with an "unsupported query" error.

#### Scenario: A free-text request is refused

- **WHEN** facetd receives a facet request with query `"golang"`
- **THEN** it returns an "unsupported query" error

#### Scenario: Facet counts for a filter

- **WHEN** facetd receives facets `["skills", "countries"]` under the filter `[["work_mode = \"remote\""]]`
- **THEN** it returns the total of open matching documents and, per facet, every value with a non-zero count among them

### Requirement: facetd shapes a facet distribution the way the jobs index does

For each requested facet facetd SHALL return at most `maxValuesPerFacet` (1200) values, the ones
with the highest counts, as the index settings make Meilisearch do (`sortFacetValuesBy: count`). For
each requested numeric facet it SHALL return the minimum and maximum over the matching documents, as
Meilisearch's `facetStats` does. The total SHALL be the exact number of open matching documents.

#### Scenario: A facet with more values than the cap

- **WHEN** 2,000 distinct `company_slug` values occur among the matching documents
- **THEN** facetd returns the 1,200 with the highest counts

#### Scenario: Numeric facet stats

- **WHEN** the requested facets include `enrichment.salary_min` and the matching documents carry values from 40,000 to 250,000
- **THEN** facetd returns min 40,000 and max 250,000 for it

### Requirement: The search client routes between Meilisearch and facetd

The search client SHALL route each eligible raw facet request (no query text) by a mode read at
runtime: `meili` sends it to Meilisearch only; `shadow` answers from Meilisearch and evaluates the
same request on facetd in the background, recording any difference; `facetd` answers from facetd and,
on an error, an unsupported filter or query, or a timeout, answers that request from Meilisearch. A
request carrying query text SHALL always go to Meilisearch. Business logic above the raw call (region
widening, disjunctive facets) SHALL run unchanged for both engines.

#### Scenario: Shadow mode never changes the answer

- **WHEN** the mode is `shadow` and facetd disagrees with Meilisearch
- **THEN** the caller receives Meilisearch's answer and a shadow difference is recorded with the request's filter

#### Scenario: facetd failure falls back per request

- **WHEN** the mode is `facetd` and facetd times out
- **THEN** the caller receives Meilisearch's answer for that request and a fallback is recorded with its reason

#### Scenario: The mode changes without a deploy

- **WHEN** the mode setting changes from `facetd` to `meili`
- **THEN** subsequent requests go to Meilisearch only, without restarting the API

### Requirement: facetd stays as fresh as the Meilisearch index

`search-drain` SHALL apply every document upsert, close, duplicate mark and delete to facetd from
the same batch it sends to Meilisearch. A failed facetd write SHALL NOT fail or delay the Meilisearch
write, and SHALL be retried so facetd converges.

#### Scenario: An upsert reaches both engines

- **WHEN** the drain submits a batch of changed documents
- **THEN** facetd reflects the new attribute values once the batch is applied

#### Scenario: facetd is down during a drain run

- **WHEN** facetd is unreachable while the drain runs
- **THEN** Meilisearch still receives the batch, the batch's job ids are queued for facetd, and a later drain pass re-derives those jobs' current state from Postgres and sends it once facetd is ready

### Requirement: An acknowledged write survives a facetd crash

facetd SHALL append every write batch to a write-ahead log, flushed to disk, before applying and
acknowledging it; on start it SHALL load the snapshot and replay the log entries newer than the
snapshot; a successful snapshot SHALL truncate the log.

#### Scenario: Crash between snapshots

- **WHEN** facetd acknowledges a write batch and is killed before the next snapshot
- **THEN** after restart the batch's changes are present

### Requirement: Readers never wait for writers

A facet request SHALL read a consistent snapshot of the index and SHALL NOT wait for a write batch
in progress.

#### Scenario: A large write batch does not stall reads

- **WHEN** a write batch of 15,000 documents is being applied
- **THEN** concurrent facet requests answer from the previous snapshot without waiting for the batch

### Requirement: facetd survives a restart without a full rebuild

facetd SHALL persist a snapshot of its index and, on start, load it and serve; a full rebuild
(through `cmd/reindex`, from the same documents Meilisearch receives) SHALL be available for a
missing or unreadable snapshot. Until it has loaded, facetd SHALL
report not-ready and refuse requests, so the router falls back.

#### Scenario: Restart with a snapshot

- **WHEN** facetd restarts with a valid snapshot on disk
- **THEN** it serves requests after loading it, without reading Postgres

#### Scenario: Not ready yet

- **WHEN** a request arrives before facetd has loaded an index
- **THEN** it answers "not ready" and the router serves the request from Meilisearch
