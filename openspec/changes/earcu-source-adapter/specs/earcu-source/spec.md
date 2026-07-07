## ADDED Requirements

### Requirement: eArcu per-company RSS crawl

The system SHALL provide an `earcu` source adapter that crawls a single eArcu career
site into the catalogue. The adapter is **board-based and single-company**: each
configured entry's board is the full careers host (e.g. `careers.cambridge.org`), and
the company is the configured `company`. For each configured board the adapter fetches
the keyless RSS feed at `https://<board>/jobs/rss` and returns one `Job` per feed
`<item>`. The crawl is keyless (no API key) and issues one feed request per board — the
full posting body is inline in the feed, so no per-posting request is required in the
common case.

#### Scenario: Board yields all feed items

- **WHEN** the adapter fetches a configured board
- **THEN** it returns one `Job` per `<item>` in that board's `/jobs/rss` feed, each
  populated with title, posting/detail URL, external id, free-text location, posted-at,
  and an HTML description

#### Scenario: Board host is used verbatim

- **WHEN** an entry's board is a careers host such as `careers.cambridge.org`
- **THEN** the adapter requests `https://careers.cambridge.org/jobs/rss` (the board is
  the host, not a subdomain slug), and the configured `company` is the job's company

### Requirement: Description body with detail-page fallback

The adapter SHALL take each job's description from the feed item — eArcu embeds the full
posting body as HTML inside the item's `<description>` (the `rssjobdesc` block) — and
sanitize it. WHEN a feed item carries no usable body, the adapter MUST fall back to the
posting detail page's schema.org `JobPosting` JSON-LD `description`, reusing the shared
`ldJobPosting`/`sanitizeHTML` helpers. A posting that yields no body from either source
is still returned (title/URL/location are sufficient), not dropped.

#### Scenario: Body comes from the feed

- **WHEN** a feed item's `<description>` contains the inline posting body
- **THEN** that body is sanitized into the job's description without any detail-page
  request

#### Scenario: Empty feed body falls back to detail JSON-LD

- **WHEN** a feed item has no inline body
- **THEN** the adapter fetches the item's detail URL and uses its `JobPosting` JSON-LD
  `description`; a failed detail fetch is non-fatal and leaves the description empty

### Requirement: Stable dedup identity

The adapter SHALL derive each job's `ExternalID` from the stable eArcu posting id — the
numeric id in the detail URL path (`/jobs/vacancy/<slug>/<id>/description/`) — so
re-crawling the same board dedups to the same catalogue row. A feed item whose link has
no extractable posting id is dropped rather than persisted with an empty key.

#### Scenario: Re-crawl dedups

- **WHEN** the same posting appears on a later crawl
- **THEN** it maps to the same `ExternalID` and updates the existing row rather than
  creating a duplicate

#### Scenario: Item without an id is dropped

- **WHEN** a feed item's link has no extractable posting id
- **THEN** the adapter omits that item rather than emitting a job with an empty
  `ExternalID`

### Requirement: Registered board-based provider

The adapter MUST be registered in `sources.All` under the provider key `earcu` and be
board-based (not boardless), so `cmd/ingest` and config validation recognise it, board
files require a board id, and it appears in the source facet like the other ATS adapters.

#### Scenario: Provider is recognised by config validation

- **WHEN** a board file names provider `earcu` with a board host
- **THEN** config validation accepts it, and an `earcu` entry that omits its board is
  rejected
