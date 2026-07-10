# backend-br-ats-adapters Specification

## Purpose
TBD - created by archiving change backend-br-ats-adapters. Update Purpose after archive.
## Requirements
### Requirement: Quickin per-company crawl

The system SHALL provide a `quickin` source adapter that crawls a single Quickin account
into the catalogue. The adapter is board-based and single-company: each configured entry's
board is the Quickin account slug (as in `jobs.quickin.io/<slug>`). The adapter resolves
the slug to the account's opaque id via `GET https://api.quickin.io/public/accounts/<slug>`,
then pages `GET https://api.quickin.io/public/<accountId>/jobs`. The listing carries the
full posting inline, so no per-posting detail request is required. The crawl is keyless.

#### Scenario: Board yields published postings

- **WHEN** the adapter fetches a configured board
- **THEN** it resolves the account id, pages the jobs listing, and returns one `Job` per
  `published` posting, populated with title, career URL, external id, location
  (city/region/country), work mode (from `workplace_type`), posted-at, and a sanitized
  HTML body (description + requirements)

#### Scenario: Unpublished postings are dropped

- **WHEN** a listing posting's `publicate` is not `published`
- **THEN** the adapter omits it

### Requirement: Mindsight per-company crawl

The system SHALL provide a `mindsight` source adapter that crawls a single Mindsight
career page into the catalogue. The adapter is board-based and single-company: each entry's
board is the tenant path segment served from `oportunidades.mindsight.com.br/<slug>`. The
adapter reads the page's Next.js `__NEXT_DATA__` `publicJobPostings` array for the
structured fields and fetches each posting's detail page for its description body. The
crawl is keyless.

#### Scenario: Listing drives the crawl, detail supplies the body

- **WHEN** the adapter fetches a configured board
- **THEN** it returns one `Job` per open (`IN_PROGRESS`) posting with title, detail URL,
  external id, location (city/state/country), work mode (from `work_model`), posted-at,
  and a description sanitized from that posting's detail-page `jobPosting.description`

#### Scenario: Closed postings are dropped

- **WHEN** a listing posting's status is not `IN_PROGRESS`
- **THEN** the adapter omits it, and a failed detail fetch is non-fatal (the posting is
  still returned, with an empty description)

### Requirement: Enlizt per-company crawl

The system SHALL provide an `enlizt` source adapter that crawls a single Enlizt tenant
(`<tenant>.enlizt.me`) into the catalogue, board-based and single-company, taking each
posting's structured fields and body from the tenant's keyless public listing and its
schema.org `JobPosting` data.

#### Scenario: Board yields tenant postings

- **WHEN** the adapter fetches a configured board
- **THEN** it returns one `Job` per open posting with title, posting URL, external id,
  location, posted-at, and a sanitized HTML body

### Requirement: Registered board-based providers

Each new adapter MUST be registered in `sources.All` under its provider key and be
board-based (not boardless), so `cmd/ingest` and config validation recognise it, board
files require a board id, and it appears in the source facet (`FilterableProviders()` /
the generated `SOURCE_VALUES`) like the other ATS adapters.

#### Scenario: Provider is recognised by config validation

- **WHEN** a board file names one of the new providers with a board id
- **THEN** config validation accepts it, and an entry that omits its board is rejected

#### Scenario: Provider appears in the source facet

- **WHEN** `FilterableProviders()` is enumerated
- **THEN** each new provider key is present, and `web/src/lib/generated/contracts.ts`
  `SOURCE_VALUES` includes it after regeneration

