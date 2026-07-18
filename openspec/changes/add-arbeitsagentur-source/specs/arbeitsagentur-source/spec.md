## ADDED Requirements

### Requirement: Arbeitsagentur enumerates postings by professional field

The system SHALL provide an `arbeitsagentur` source adapter that queries the Bundesagentur für
Arbeit search API at `https://rest.arbeitsagentur.de/jobboerse/jobsuche-service/pc/v4/jobs`,
attaching the static public `X-API-Key: jobboerse-jobsuche` header. The adapter SHALL filter by the
`berufsfeld` professional-field value carried as the board file entry's `board`, and SHALL paginate
(`size=100`, `page=1..N`) until a page returns fewer than `size` postings, the cumulative count
reaches `maxErgebnisse`, or the API's pagination depth cap is reached. A run SHALL bound its window
with a `veroeffentlichtseit` (published-within-N-days) filter so each crawl is an incremental window.

The adapter is board-based (the `board` is the `berufsfeld`) and multi-company (each posting's
company is its `arbeitgeber`); it carries neither the boardless nor the aggregator marker. The public
key is a constant, so the provider registers unconditionally.

#### Scenario: A berufsfeld board is paginated

- **WHEN** the adapter crawls a board whose `board` is a `berufsfeld` value
- **THEN** it requests the search API with that `berufsfeld` and the `X-API-Key` header, advancing
  `page` until the results are exhausted or the depth cap is reached

### Requirement: Arbeitsagentur keeps only first-party postings

The adapter SHALL drop every search result that carries a non-empty `externeUrl` — those postings are
re-listed from other boards and apply off-site — and keep only the agency's own first-party postings.

#### Scenario: An externeUrl re-list is dropped

- **WHEN** a search result carries a non-empty `externeUrl`
- **THEN** the adapter yields no `Job` for it and continues with the remaining results

### Requirement: Arbeitsagentur maps a first-party posting to a Job with a scraped description

For each kept posting the adapter SHALL fetch the server-rendered detail page
`https://www.arbeitsagentur.de/jobsuche/jobdetail/<refnr>` and map the posting to a normalized `Job`
carrying `refnr` as its `ExternalID`, the detail page URL as its canonical URL, `titel` as its title,
`arbeitgeber` as its company, the `arbeitsort` (`ort`, `region`, `land`) as its location,
`aktuelleVeroeffentlichungsdatum` as its posted-at, and the detail page's `Stellenbeschreibung` (or,
when that block is absent, the page's meta-description summary) as its sanitized description.

A detail-page fetch that fails or yields no description SHALL NOT drop the posting — it is emitted with
an empty description — and a single failed page SHALL NOT abort the crawl.

#### Scenario: A first-party posting maps to a job

- **WHEN** the adapter keeps a first-party posting and fetches its detail page
- **THEN** it yields one `Job` with `ExternalID` set to `refnr`, the jobdetail page as the URL, the
  title, company `arbeitgeber`, the `arbeitsort` location, the publish date, and the scraped
  `Stellenbeschreibung` as the description

#### Scenario: A posting whose detail page yields no description is still emitted

- **WHEN** a kept posting's detail page fetch fails or carries no description block
- **THEN** the adapter still yields the `Job` (with an empty description) and continues the crawl
