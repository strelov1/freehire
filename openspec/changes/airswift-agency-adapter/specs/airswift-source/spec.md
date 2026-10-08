## ADDED Requirements

### Requirement: Airswift sitemap-enumerated crawl

The system SHALL provide an `airswift` source adapter that enumerates
`https://www.airswift.com/sitemap.xml` (a flat urlset) to discover every job-detail URL
matching the canonical `/jobs/<slug>-<id>` shape, fetches each detail page, and parses
the embedded schema.org `JobPosting` ld+json. The adapter is **boardless**.

#### Scenario: Sitemap yields every canonical job URL

- **WHEN** the adapter crawls its single configured entry
- **THEN** it returns one `Job` per canonical `/jobs/<slug>-<id>` URL in the sitemap
  whose detail page is live (non-expired) and carries a parseable `JobPosting`

#### Scenario: A broken sitemap errors the crawl

- **WHEN** the sitemap cannot be fetched or decoded
- **THEN** `Fetch` returns an error rather than an empty success

### Requirement: Expired postings are dropped, not marked unreadable

The adapter SHALL detect a confirmed-expired posting — a detail page that renders a
`c-jobs-article-expired` element instead of a `JobPosting` block — and omit it from the
result entirely, the same way a confirmed-gone 404/410 is handled elsewhere in this
codebase. It MUST NOT emit an `unreadableDetail` stub for a confirmed-expired posting.

#### Scenario: An expired posting is silently omitted

- **WHEN** a detail page renders the `c-jobs-article-expired` marker
- **THEN** that posting is absent from the result, and is not counted as `Unreadable`

#### Scenario: A genuinely unparseable (non-expired) page still gets the standard stub

- **WHEN** a detail page fetches successfully, carries no expired marker, and carries no
  parseable `JobPosting` block
- **THEN** the adapter returns an `unreadableDetail` stub for that posting (the existing
  convention for an ambiguous failure)

### Requirement: Self-contained single-company identity

The adapter SHALL set every `Job`'s `Company` to the constant `"Airswift"`, because the
agency's `hiringOrganization.name` is confirmed to be that literal string on every live
posting (standard recruitment-agency anonymization) — this is not an aggregator.

#### Scenario: Company is always Airswift

- **WHEN** any live posting is mapped to a `Job`
- **THEN** its `Company` is `"Airswift"`

### Requirement: Resilient per-posting parsing

The adapter SHALL fetch job-detail pages through the shared bounded-fan-out helper
(`fetchDetails`/`defaultDetailWorkers`), isolating a single detail-page failure (fetch
error, expired, or malformed) from the rest of the crawl.

#### Scenario: One bad detail page does not fail the crawl

- **WHEN** one job-detail URL fails to fetch (a non-404/410 error)
- **THEN** the adapter emits an `unreadableDetail` stub for that one posting and still
  returns the successfully parsed postings from the rest of the crawl

#### Scenario: A confirmed-gone detail page is dropped, not marked unreadable

- **WHEN** a job-detail URL answers 404 or 410
- **THEN** the adapter drops that posting entirely, consistent with `bayt.go`'s existing
  404/410 handling
