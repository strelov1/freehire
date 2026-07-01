## ADDED Requirements

### Requirement: Traffit tenant crawl

The system SHALL provide a `traffit` source adapter that crawls one Traffit tenant's
public job list into the catalogue. The board id is the tenant subdomain, and the
adapter fetches `https://<board>.traffit.com/public/an/list/` — a keyless public JSON
endpoint. The adapter is board-based (it requires a board id) and appears in the source
facet.

#### Scenario: Board yields all live postings

- **WHEN** the adapter fetches a configured tenant board
- **THEN** it returns one `Job` per posting in the tenant's list, with the posting's
  title, HTML description (sanitized), free-text location derived from the posting's
  structured geolocation, apply/detail URL, and posted-at timestamp

#### Scenario: Stable dedup identity

- **WHEN** the adapter maps a posting to a `Job`
- **THEN** the `ExternalID` is the posting's stable Traffit advert id, so re-crawling
  the same tenant dedups to the same catalogue row

### Requirement: Paginated collection

The endpoint caps a page at 100 items and defaults to 10, so the adapter SHALL page
through the tenant's postings until it has collected the reported total (`count`) or a
page returns no items — a single-page fetch would silently truncate boards larger than
one page.

#### Scenario: Board larger than one page

- **WHEN** a tenant has more postings than one page (e.g. 167 postings)
- **THEN** the adapter returns every live posting, not just the first page

#### Scenario: Guard against a never-ending feed

- **WHEN** the endpoint keeps returning a full page without ever reaching `count`
- **THEN** the adapter stops after a bounded number of pages rather than looping forever

### Requirement: Harvest tenant validation

The system SHALL provide a harvest prober for the `traffit` provider that validates a
candidate tenant slug against the list endpoint. A real tenant returns JSON with a job
count; a non-tenant subdomain returns an HTML placeholder. The prober keeps a candidate
only when the endpoint returns parseable JSON with at least one live posting, and skips
any other candidate without aborting the harvest.

#### Scenario: Live tenant kept

- **WHEN** the prober probes a slug whose list endpoint returns JSON with live postings
- **THEN** it reports the tenant as live with its open-job count

#### Scenario: Non-tenant skipped

- **WHEN** the prober probes a slug whose subdomain returns the HTML placeholder (no JSON)
- **THEN** it skips the candidate silently and continues with the rest
