## ADDED Requirements

### Requirement: Orion recruitment API paginated crawl

The system SHALL provide an `orion` source adapter that pages
`https://www.orionjobs.com/api/recruitment/job/data/?folder=uk&hasexpired=false&page=N`
to enumerate every currently-open posting. The adapter is **boardless**: one crawl covers
the whole site, no per-tenant board id. The crawl is keyless (no API key, no auth
header).

#### Scenario: Pagination is followed to exhaustion

- **WHEN** the adapter crawls its single configured entry
- **THEN** it advances through pages until a page returns no items (or the response's
  `pagination.to` reaches `pagination.total`), so postings past the first page are not
  silently dropped

#### Scenario: A broken first page errors the crawl

- **WHEN** the first page request fails
- **THEN** `Fetch` returns an error rather than an empty success

### Requirement: No separate detail fetch

The adapter SHALL map each `Job` directly from the listing response, because the listing
payload already carries the full HTML description — unlike a hydrating adapter (e.g.
`emagine.go`), no per-posting detail request is made.

#### Scenario: Description comes from the listing item

- **WHEN** a listing item carries a non-empty `description.value`
- **THEN** the resulting `Job`'s `Description` is populated from that field alone, with
  no additional HTTP request for the posting

### Requirement: Fixed agency company identity

The adapter SHALL set every `Job`'s `Company` to the constant `"Orion Group"`, because
Orion's API never names a real end client (standard recruitment-agency anonymization,
confirmed live) — this is a single-company source, not an aggregator.

#### Scenario: Company is always Orion Group

- **WHEN** any posting is mapped to a `Job`
- **THEN** its `Company` is `"Orion Group"` regardless of the posting's own content

### Requirement: Stable dedup identity

The adapter SHALL derive each job's `ExternalID` from the listing item's integer `id`
field, not its `slug` or `url` (which can change when a posting's title is edited), so
re-crawling dedups to the same catalogue row.

#### Scenario: Re-crawl dedups

- **WHEN** the same posting `id` is seen on a later crawl
- **THEN** it maps to the same `ExternalID` and updates the existing row rather than
  creating a duplicate
