# nofluffjobs-source Specification

## Purpose
TBD - created by archiving change add-nofluffjobs-source. Update Purpose after archive.
## Requirements
### Requirement: NoFluffJobs maps its streamed listing to jobs

The system SHALL provide a `nofluffjobs` source adapter that reads the
`https://nofluffjobs.com/api/posting` listing (a single large JSON document, fetched via streaming
past the size-capped JSON getter) and maps each posting to a normalized `Job`. The `Job` SHALL carry
the posting `id` as its `ExternalID`, `https://nofluffjobs.com/job/<url>` (the ASCII slug) as its
canonical URL, the `title`, the `name` as its company, the `location.places[0]` city/country (or
remote) as its location with `fullyRemote` as its remote flag, the `posted` epoch-milliseconds value
as its posted-at, the canonicalized `technology` as its skills, and the mapped `seniority` as its
seniority.

The adapter is **boardless** (nofluffjobs.com is one API with no per-tenant board) and an
**aggregator** (company per posting), so it stays in the source facet. The configured board file
entry carries no `board` value.

#### Scenario: A listing posting maps to a job

- **WHEN** the adapter reads a posting with an `id`, a `url` slug, a `title`, a `name`, a `posted`
  timestamp, a `technology`, and a `seniority`
- **THEN** it yields one `Job` with `ExternalID` = `id`, the `job/<url>` canonical URL, the title,
  the company, the location/remote flag, the posted-at from `posted`, the canonicalized skills, and
  the mapped seniority

### Requirement: NoFluffJobs hydrates new postings' descriptions from the detail endpoint

The listing carries no description, so the adapter SHALL implement the hydrating `FetchNew` path: for
a posting the seen-predicate reports as **new**, it SHALL fetch `https://nofluffjobs.com/api/posting/<url>`
and set the `Job` description to the sanitized `details.description` combined with
`requirements.description`; for a posting already ingested (seen), it SHALL emit the list-only `Job`
without a detail request so the pipeline refreshes liveness without wiping the previously-hydrated body.
A detail request that fails SHALL fall back to the list-only `Job` rather than dropping the posting.
The plain `Fetch` path SHALL map the listing without any detail request.

#### Scenario: A new posting is hydrated with its description

- **WHEN** `FetchNew` runs and a posting's `id` is not in the seen set
- **THEN** the adapter fetches that posting's detail and the resulting `Job` carries the sanitized
  description assembled from the detail's `details.description` and `requirements.description`

#### Scenario: An already-seen posting is not detail-fetched

- **WHEN** `FetchNew` runs and a posting's `id` is already in the seen set
- **THEN** the adapter emits the list-only `Job` for it and makes no detail request

### Requirement: NoFluffJobs drops unusable postings

The adapter SHALL drop any posting with no `id`, no `url` slug, or no company — an empty dedup key,
an unbuildable canonical URL, or an empty company (which breaks the public slug). A single dropped
posting SHALL NOT abort the crawl.

#### Scenario: A posting with no company is dropped

- **WHEN** a posting has an empty `name`
- **THEN** the adapter yields no `Job` for it and continues mapping the rest

