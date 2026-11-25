# search-index-exclusions Specification

## Purpose
What is kept out of the jobs index, and how a filter Meilisearch rejects degrades instead of failing the request. Split out of job-search.

## Requirements

### Requirement: Duplicate jobs are hidden by indexed status

A posting newly marked as a duplicate SHALL have its existing Meilisearch document marked `is_duplicate = true` through the durable state-checked removal queue rather than physically deleted. Updates SHALL preserve text and vectors, change existing documents only, and be idempotent. All job searches, vector/federated queries, facets, distinct counts and job sitemaps SHALL exclude marked duplicates before pagination. Legacy documents lacking the marker SHALL remain eligible. The internal marker SHALL not enter the public job response or facet vocabulary.

The index document built from a database row SHALL carry its current duplicate status. Replacing an open canonical document SHALL clear stale closure and duplicate markers. A job released from duplication while an update is in flight SHALL be requeued for indexing on acknowledgment. Missing/private rows SHALL still be physically removed. Full rebuilds SHALL continue to omit duplicate and closed documents and discard obsolete indexes.

A link import's duplicate transition SHALL use its existing atomic removal outbox and SHALL NOT submit a separate direct index deletion.

#### Scenario: A duplicate retains its indexed content
- **WHEN** a searchable posting becomes a duplicate and the status wave completes
- **THEN** its text and vectors remain stored but it contributes no hits, facets, distinct counts or sitemap entries

#### Scenario: An absent duplicate document creates no skeleton
- **WHEN** a duplicate status wave includes a job absent from the index
- **THEN** no document is created

#### Scenario: A released duplicate returns to search
- **WHEN** a duplicate becomes canonical before or during its status wave
- **THEN** it is queued for current-content replacement and becomes searchable with stale markers cleared

#### Scenario: A private or missing duplicate is physically removed
- **WHEN** a queued duplicate is private or its database row is missing
- **THEN** its document is physically removed regardless of duplicate status

#### Scenario: Link import has one durable duplicate update
- **WHEN** reimporting a previously searchable generic URL identifies an existing canonical posting
- **THEN** the marking transaction queues the duplicate and the importer submits no direct engine deletion

### Requirement: Category-unresolved jobs are excluded from the index, with a confirmed-technical carve-out

The system SHALL exclude a job from the searchable index (both the incremental
push and the full rebuild) when its category is unresolved by both the
deterministic title dictionary and enrichment (empty or the catch-all
`"other"`) — this keeps the undifferentiated non-tech bulk a broad ATS crawl
brings in (painters, stockers, drivers, and similarly unfiltered roles) out of
a catalogue no category filter, and often no keyword search, was ever meant to
surface. This exclusion SHALL NOT apply when the job's `is_tech` signal is
confidently `true`: a confirmed-technical posting carries meaningful signal
even without a resolved sub-category, so it is not the undifferentiated bulk
the exclusion targets, and it SHALL remain searchable regardless of whether
its category ever resolves further.

#### Scenario: A job with no resolved category and no tech signal is excluded
- **WHEN** a job's category is empty, its enrichment carries no category or
  `"other"`, and its `is_tech` is unknown or `false`
- **THEN** the job is excluded from the search index

#### Scenario: A confirmed-technical job with no resolved category remains searchable
- **WHEN** a job's category is empty, its enrichment carries no category or
  `"other"`, but its `is_tech` is `true`
- **THEN** the job is included in the search index

#### Scenario: A resolved category always includes the job regardless of is_tech
- **WHEN** a job's category (from the title dictionary or enrichment) is
  resolved to a specific, non-`"other"` value
- **THEN** the job is included in the search index

### Requirement: A filter Meilisearch rejects degrades rather than failing the request

When a search request to `GET /api/v1/jobs/search` (or its agent variant) carries at least
one facet or scalar filter and the underlying search fails because Meilisearch rejected the
filter itself (as opposed to a general engine or transport failure), the system SHALL retry
the search once with the dynamic filter dropped entirely — query text, sort, vector ranking,
and pagination unchanged. On a successful retry, every query param that is part of the known
filter vocabulary and was present on the request SHALL be reported in `meta.ignored_params`,
using the same shape (and merged with) the report an unrecognized param already receives.

This SHALL NOT apply when the request carried no dynamic filter to blame, or when the
underlying failure is not classified as a filter rejection (a genuine engine or transport
failure) — such a request SHALL fail exactly as it does today.

The system SHALL NOT attempt to identify which single filter param caused the rejection.
Every active filter param SHALL be reported, since Meilisearch's own error does not reliably
name the offending attribute.

#### Scenario: A filter Meilisearch cannot yet honor degrades to an unfiltered result

- **WHEN** a search request carries a facet filter and Meilisearch rejects the query because
  that filter cannot currently be evaluated
- **THEN** the response is `200` with results as if the filter had not been sent
- **AND** `meta.ignored_params` names every filter param the request carried

#### Scenario: A request with no filter never triggers the retry

- **WHEN** a search request carries no facet or scalar filter and fails
- **THEN** the response reports the failure exactly as it does today, with no retry attempted

#### Scenario: A non-filter failure is not degraded

- **WHEN** a search request fails for a reason that is not a filter rejection (e.g. the search
  engine is unreachable)
- **THEN** the response reports the failure exactly as it does today, with no retry attempted

#### Scenario: A retry that also fails reports the original outcome

- **WHEN** a search request carries a filter, the primary search is rejected as a filter
  error, and the retry without the filter also fails
- **THEN** the response reports a failure, not a degraded success

