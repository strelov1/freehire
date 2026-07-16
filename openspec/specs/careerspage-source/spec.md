# careerspage-source Specification

## Purpose

Crawl one careers-page.com tenant's server-rendered job listing into the
catalogue, mapping each posting's schema.org `JobPosting` detail into a normalized
`Job`.

## Requirements

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

The adapter SHALL fetch each posting's detail page (which holds the description and
structured fields) under a bounded concurrent worker pool, dropping only the
postings whose detail fetch fails or carries no `JobPosting`, without aborting the
board.

#### Scenario: One bad detail page does not fail the board

- **WHEN** a single posting's detail page fails to fetch or lacks a `JobPosting`
  `ld+json` block
- **THEN** that posting is skipped and the rest of the board's postings are still
  returned

### Requirement: Rate-paced crawl for full single-run coverage

The `careerspage` crawl SHALL hold its aggregate outbound request rate under a bounded
limit shared across all of a run's requests — the paginated listing and every posting's
detail fetch, across every configured board — so that one run stays within
careers-page.com's per-IP rate-limit window and collects every posting. The rate cap MUST
be independent of the detail worker-pool size, so that widening or narrowing concurrency
does not change the request rate. This prevents an earlier or larger board from spending
the window budget and starving later boards in the same run.

#### Scenario: Later boards do not starve behind an earlier board

- **WHEN** a run crawls several careerspage boards over one shared egress and an earlier
  board issues many detail requests
- **THEN** the aggregate request rate stays under the limit and later boards still collect
  their full set of postings in the same run, rather than being rate-limited to a partial set

#### Scenario: Rate is governed by the pacer, not the worker pool

- **WHEN** the detail fan-out runs under its bounded worker pool
- **THEN** the outbound request rate is bounded by the shared pacer regardless of the pool
  size, so no burst exceeds the window budget
