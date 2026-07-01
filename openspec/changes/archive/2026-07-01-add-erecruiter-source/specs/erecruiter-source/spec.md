## ADDED Requirements

### Requirement: eRecruiter company-board crawl

The system SHALL provide an `erecruiter` source adapter that crawls one company's public
eRecruiter "Strona Kariera" board into the catalogue. The board id is the company's `cfg`
config token (32-hex), and the adapter fetches the keyless public endpoint
`https://skk.erecruiter.pl/GetHtml.ashx?cfg=<cfg>&grid=rows&pn=<page>`. The adapter is
board-based (it requires a board id) and appears in the source facet.

#### Scenario: Board yields all live postings

- **WHEN** the adapter fetches a configured company board
- **THEN** it returns one `Job` per posting on the board, each with the posting's title,
  HTML description (sanitized), free-text location, apply/detail URL, and — when the
  board exposes it — a posted-at timestamp

#### Scenario: Stable dedup identity

- **WHEN** the adapter maps a board row to a `Job`
- **THEN** the `ExternalID` is the posting's stable `externalJobOfferId`, so re-crawling
  the same company board dedups to the same catalogue row

### Requirement: Two-step list then detail fetch

The adapter SHALL fetch each posting's `Offer.aspx` detail page and parse its description
and canonical detail URL, so a persisted posting carries a real body rather than only a
one-line title. This is required because the list endpoint returns only a row summary
(title, category, city, and the id fields offerId, externalJobOfferId,
externalJobOfferRegionId, comId); the full description lives on the per-offer detail page,
addressed by those id fields plus the board `cfg`.

#### Scenario: Detail supplies the description

- **WHEN** the adapter has a list row and fetches its detail page
- **THEN** the resulting `Job` carries the posting's full sanitized description and the
  `Offer.aspx` URL as its canonical detail URL

#### Scenario: Detail page missing

- **WHEN** a posting's detail page is gone (the row references a closed offer)
- **THEN** the adapter skips that posting without aborting the rest of the board

### Requirement: Paginated collection bounded by reported total

The adapter SHALL page through the board until it has collected the reported total (or a
page returns no offer rows), and SHALL stop after a bounded number of pages so a
pathological feed can never loop forever. The list endpoint returns a fixed page size (20
rows) and, on the first page, a marker row whose `tr` attribute is the board's total
result count; a single-page fetch would otherwise silently truncate a board larger than
one page.

#### Scenario: Board larger than one page

- **WHEN** a company has more postings than one page (e.g. 32 postings, page size 20)
- **THEN** the adapter returns every live posting, not just the first page

#### Scenario: Guard against a never-ending feed

- **WHEN** the endpoint keeps returning full pages without ever reaching the total
- **THEN** the adapter stops after a bounded number of pages rather than looping forever

### Requirement: Harvest cfg discovery and validation

The system SHALL provide a harvest tool for the `erecruiter` provider that resolves a
company's `cfg` board token from the company's own careers page (fetch the page and
extract the `https://skk.erecruiter.pl/Code.ashx?cfg=<hex>` widget reference), then
live-validates the token against the board endpoint. It keeps a candidate only when the
board endpoint returns parseable offer rows, and skips any other candidate without
aborting the harvest. Valid tokens are emitted as ready-to-paste `sources/erecruiter.yml`
entries (`company` + `board`=cfg). This is the discovery bridge for companies known only
by an eRecruiter apply-form `WebID`, which carries no `cfg`.

#### Scenario: Careers page exposes a cfg

- **WHEN** the harvester fetches a company careers page that embeds the eRecruiter widget
- **THEN** it extracts the `cfg` token, confirms the board endpoint returns live offer
  rows, and emits a board-file entry for that company

#### Scenario: No eRecruiter widget on the page

- **WHEN** a candidate careers page does not reference `skk.erecruiter.pl/Code.ashx?cfg=`
- **THEN** the harvester skips the candidate silently and continues with the rest
