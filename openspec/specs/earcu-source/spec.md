# earcu-source Specification

## Purpose
TBD - created by archiving change earcu-source-adapter. Update Purpose after archive.
## Requirements
### Requirement: eArcu per-company RSS crawl

The system SHALL provide an `earcu` source adapter that crawls a single eArcu career
site into the catalogue. The adapter is **board-based and single-company**: each
configured entry's board is the full careers host (e.g. `careers.cambridge.org`), and
the company is the configured `company`. For each configured board the adapter fetches
the keyless RSS feed at `https://<board>/jobs/rss` and returns one `Job` per feed
`<item>`. The crawl is keyless (no API key). The feed names every open posting but only
previews it — its inline body is a ~1,000-character excerpt ending in "...", and its
metadata prefix carries a place on few tenants — so the adapter is a `HydratingSource`:
a posting the catalogue does not hold costs one detail-page request, and a stored one is
re-listed as a `SeenRefresh` with no request.

#### Scenario: Board yields all feed items

- **WHEN** the adapter fetches a configured board
- **THEN** it returns one `Job` per `<item>` in that board's `/jobs/rss` feed, each
  populated with title, posting/detail URL, external id, free-text location, posted-at,
  and an HTML description

#### Scenario: Board host is used verbatim

- **WHEN** an entry's board is a careers host such as `careers.cambridge.org`
- **THEN** the adapter requests `https://careers.cambridge.org/jobs/rss` (the board is
  the host, not a subdomain slug), and the configured `company` is the job's company

### Requirement: Body and place from the detail page

For a posting the catalogue does not hold, the adapter SHALL read the posting's detail
page. The description MUST be the page's visible posting body (`div.PosDescriptionText`,
else `div.earcu_posdescriptionContainer`), falling back to the schema.org `JobPosting`
`description`, and only then to the feed's preview. The location MUST be the
`JobPosting` `jobLocation` addresses ("Locality, Region, Country", repeated parts
dropped, several places joined by "; "), falling back to the page's summary row whose
label ends in "Location", then to the feed prefix's `Location` field, then its `Country`
field. `Job.Countries` MUST be set from the structured `addressCountry` values (or the
feed's `Country` field) through `location.NormalizeCountry`, never from free text. A
new posting whose detail fetch fails MUST NOT be returned: stored with the feed's preview
it would be seen and never read again. Left out, it stays new and the next crawl retries
it. A 404/410 drops the posting the same way. Neither fails the board.

#### Scenario: Place and body come from the detail page

- **WHEN** a new posting's feed prefix states no place and its detail page carries a
  `JobPosting` with `jobLocation.address.addressCountry`
- **THEN** the job's location is the address and its countries the normalized country,
  and its description is the page's full body rather than the feed preview

#### Scenario: Stored posting costs no request

- **WHEN** a listed posting is already in the catalogue
- **THEN** the adapter emits it as a `SeenRefresh` and requests no detail page

#### Scenario: Failed detail fetch defers the posting

- **WHEN** a new posting's detail fetch fails
- **THEN** the posting is not returned, the board's other postings are, and the next
  crawl requests that posting's detail page again

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

