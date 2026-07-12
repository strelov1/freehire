## ADDED Requirements

### Requirement: NEOGOV adapter ingests career-site postings

The system SHALL ingest jobs from NEOGOV career sites (governmentjobs.com and its
education vertical schooljobs.com) through a `neogov` `Source` adapter registered by
provider key. The board SHALL be `"<domain>/<agency>"` (e.g.
`schooljobs.com/cochisecollege`), and an invalid board (missing domain or agency)
SHALL fail fast with an error.

The listing endpoint is a Knockout SPA whose job cards are served only when the
request carries an `X-Requested-With: XMLHttpRequest` header; without it the endpoint
returns the empty JS shell. The adapter SHALL send that header for the listing,
SHALL paginate until a page yields no new cards or the header count is reached, and
SHALL parse each `li.list-item[data-job-id]` card for its title, detail URL, and
location.

#### Scenario: Adapter parses listing cards into jobs

- **WHEN** the adapter fetches an agency's listing fragment with the XHR header
- **THEN** it yields one job per `li.list-item[data-job-id]` card, carrying the card's
  title, absolute detail URL, location, and the platform's `data-job-id` as the
  external id

#### Scenario: Invalid board fails fast

- **WHEN** a board string lacks a `<domain>/<agency>` split
- **THEN** the adapter returns an error and yields no jobs

### Requirement: NEOGOV adapter stores the full detail-page description

The adapter SHALL fetch each listing card's detail page and yield the full posting
body — the `#details-info` (`.fr-view`) container that holds the job's Definition,
Minimum Qualifications, and Supplemental Information sections — as the job
`description`, in place of the listing card's teaser snippet. The detail page is
server-rendered, so the detail fetch SHALL be a plain GET that does NOT send the
`X-Requested-With` header the listing requires. The description SHALL be sanitized
HTML (per the source-ingest sanitized-HTML requirement), assembled from the
container's inner markup, not the plain-text snippet.

Detail fetches SHALL be bounded so a single board cannot issue unbounded concurrent
requests. A detail fetch that fails or yields an empty body SHALL degrade to the
listing card's snippet rather than dropping the job or storing a blank description.

#### Scenario: Full body replaces the listing snippet

- **WHEN** the adapter processes a listing card whose detail page carries the full
  posting body in `#details-info`
- **THEN** the yielded job description is the sanitized HTML of that container,
  containing the full sections rather than only the card's opening paragraph

#### Scenario: Detail fetch failure degrades to the snippet

- **WHEN** a card's detail page fails to fetch or its `#details-info` container is
  absent or empty
- **THEN** the adapter yields the job with the listing card's snippet as its
  description rather than dropping the job or storing a blank description

### Requirement: Existing NEOGOV rows backfill through re-ingest

The change SHALL NOT require a dedicated backfill worker. Because `UpsertJob`
overwrites a stored description with a non-empty incoming one and the resulting
`content_hash` change re-pushes the document to the search index, a normal
`cmd/ingest sources/neogov.yml` crawl SHALL correct every existing NEOGOV row in
place.

#### Scenario: Re-ingest replaces a stored snippet in place

- **WHEN** an existing NEOGOV job whose stored description is the old listing snippet
  is re-crawled by the updated adapter
- **THEN** `UpsertJob` overwrites the description with the full detail body and reports
  the row as content-changed, so it is re-indexed without a separate backfill script
