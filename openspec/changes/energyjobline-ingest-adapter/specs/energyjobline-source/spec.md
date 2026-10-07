## ADDED Requirements

### Requirement: EnergyJobline sitemap-enumerated crawl

The system SHALL provide an `energyjobline` source adapter that enumerates
`https://www.energyjobline.com/sitemap.xml` — a sitemap **index** of sub-sitemaps — to
discover every job-detail URL, fetches each detail page, and parses the embedded
schema.org `JobPosting` JSON-LD. The adapter is **boardless**: one crawl covers the whole
site, with no per-tenant board id. The crawl is keyless (no API key).

#### Scenario: Sitemap index yields every posting

- **WHEN** the adapter crawls its single configured entry
- **THEN** it resolves the sitemap index to its sub-sitemaps, collects every job-detail
  URL across them, and returns one `Job` per URL whose detail page carries a parseable
  `JobPosting`

#### Scenario: A broken sitemap index errors the crawl

- **WHEN** the top-level sitemap index cannot be fetched or decoded
- **THEN** `Fetch` returns an error rather than an empty success, so a broken enumeration
  is a loud signal and not silently treated as "zero jobs today"

### Requirement: Self-contained aggregator company identity

The adapter MUST read each posting's company from the JSON-LD `hiringOrganization.name`
rather than a configured board company, because EnergyJobline is a multi-company
aggregator where each posting carries its own employer — the same resolution `bayt.go`/
`gulftalent.go` already use. The adapter is registered as an aggregator-marker source so
its jobs are stored under their own company identity and included in the source facet.

#### Scenario: Company comes from the posting

- **WHEN** a detail page's `JobPosting` names a `hiringOrganization`
- **THEN** that organization's `name` (trimmed) is the job's company

#### Scenario: Posting without a resolvable employer is marked unreadable, not dropped

- **WHEN** a detail page has no resolvable `hiringOrganization.name` (empty, or the
  `JobPosting` block itself is missing/unparseable)
- **THEN** the adapter returns an `unreadableDetail` stub for that posting (proving the
  URL was reached) instead of a job with a guessed or blank company, and instead of
  silently omitting it

### Requirement: Stable dedup identity

The adapter SHALL derive each job's `ExternalID` from the stable identifier in the
job-detail URL, so re-crawling the same sitemap dedups to the same catalogue row.

#### Scenario: Re-crawl dedups

- **WHEN** the same posting URL is seen on a later crawl
- **THEN** it maps to the same `ExternalID` and updates the existing row rather than
  creating a duplicate; a URL with no extractable id is skipped during enumeration rather
  than ingested with an empty key

### Requirement: Resilient per-posting parsing

The adapter SHALL fetch job-detail pages through the shared bounded-fan-out helper
(`fetchDetails`/`defaultDetailWorkers`), isolating a single detail-page failure from the
rest of the crawl: one unreachable or malformed posting must not abort an otherwise
healthy run.

#### Scenario: One bad detail page does not fail the crawl

- **WHEN** one job-detail URL fails to fetch or carries no parseable `JobPosting`
- **THEN** the adapter emits an `unreadableDetail` stub for that one posting and still
  returns the successfully parsed postings from the rest of the crawl
