## ADDED Requirements

### Requirement: careers-page.com tenant crawl

The system SHALL provide a `careerspage` source adapter that crawls one
careers-page.com tenant's public job listing into the catalogue. The board id is
the tenant subdomain, and the adapter fetches the tenant's server-rendered
listing at `https://<board>.careers-page.com` and each posting's detail page. The
crawl is keyless. The adapter is board-based (it requires a board id) and appears
in the source facet.

#### Scenario: Board yields all live postings

- **WHEN** the adapter fetches a configured tenant board
- **THEN** it returns one `Job` per posting linked from the tenant's listing, with
  the posting's title, HTML description (sanitized), free-text location, apply/detail
  URL, and posted-at timestamp taken from the detail page's schema.org `JobPosting`

#### Scenario: Stable dedup identity

- **WHEN** the adapter maps a posting to a `Job`
- **THEN** the `ExternalID` is the posting's stable careers-page.com job UUID (from
  its `/jobs/<uuid>` URL), so re-crawling the same tenant dedups to the same
  catalogue row

#### Scenario: Company falls back to the configured entry

- **WHEN** a posting's `hiringOrganization.name` is present in the JSON-LD
- **THEN** it is used as the job's company, otherwise the configured board company
  is used

### Requirement: Paginated listing collection

The listing is paginated via `?page=N`, so the adapter SHALL page through the
tenant's listing until a page yields no new job links, collecting every posting —
a single-page fetch would silently truncate boards larger than one page.

#### Scenario: Board larger than one page

- **WHEN** a tenant has more postings than fit on the first listing page
- **THEN** the adapter returns every live posting, not just the first page

#### Scenario: Guard against a never-ending listing

- **WHEN** paging never reaches an empty/repeat page
- **THEN** the adapter stops after a bounded number of pages rather than looping
  forever

### Requirement: Per-posting detail isolation

The description and structured fields live on each posting's detail page, so the
adapter SHALL fetch details under a bounded concurrent worker pool and drop only
the postings whose detail fetch fails or carries no `JobPosting`, without aborting
the board.

#### Scenario: One bad detail page does not fail the board

- **WHEN** a single posting's detail page fails to fetch or lacks a `JobPosting`
  `ld+json` block
- **THEN** that posting is skipped and the rest of the board's postings are still
  returned
