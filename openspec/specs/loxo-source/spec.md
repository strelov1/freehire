# loxo-source Specification

## Purpose
TBD - created by archiving change add-loxo-adapter. Update Purpose after archive.
## Requirements
### Requirement: Loxo careers-board crawl

The system SHALL provide a `loxo` source adapter that crawls one Loxo agency's
public careers board into the catalogue. The board id is the careers URL without
scheme (e.g. `fitnext.app.loxo.co/fitnext` or `app.loxo.co/agile-recruiter`), so
one adapter uniformly covers Loxo's host variants (agency subdomain, bare
`app.loxo.co`, and regional pods); the adapter derives the origin from the board
host. The crawl is keyless and board-based, and appears in the source facet.

#### Scenario: Board yields all live postings

- **WHEN** the adapter fetches a configured Loxo board
- **THEN** it returns one `Job` per posting linked as `/job/<base64>` from the
  board's server-rendered listing page

#### Scenario: Host variant is honored

- **WHEN** a board id names an agency subdomain, the bare `app.loxo.co` host, or a
  regional pod host
- **THEN** the adapter fetches the listing and resolves each posting URL against
  that same host

### Requirement: Loxo detail mapping

The adapter SHALL map each posting's detail page to a `Job` using the page title
for the role name (stripped of the trailing ` | <agency>` suffix), the embedded
`<script type="application/json">` blob for the HTML description, and the
canonical `<host>/job/<base64>` as the apply/detail URL. Location and remote are
best-effort from the detail DOM and left empty when absent — the adapter never
guesses geography.

#### Scenario: Description comes from the embedded JSON

- **WHEN** the detail page carries the embedded JSON blob with a `description`
- **THEN** the `Job` description is that HTML, and the title is the page title
  with the ` | <agency>` suffix removed

#### Scenario: Missing structured location resolves to nothing

- **WHEN** a posting exposes no parseable location on the detail page
- **THEN** the `Job` location/remote are left empty rather than inferred

### Requirement: Stable Loxo dedup identity

The adapter SHALL set each `Job`'s `ExternalID` to the decoded base64 job id,
which is the platform's stable `<agency_id>-<slug>` pair, so re-crawling the same
board dedups to the same catalogue row.

#### Scenario: Re-crawl dedups to one row

- **WHEN** the same posting is crawled on two runs
- **THEN** both map to the same `ExternalID` (`<agency_id>-<slug>`) and upsert to a
  single catalogue row

### Requirement: Hub-based employer attribution

The adapter SHALL honor the `CompanyEntry.Hub` flag for employer attribution,
because a Loxo board is an agency hosting many clients' vacancies. When `Hub` is
set it resolves the client employer from the posting only on an explicit delimiter
in the title (e.g. `— Client` / `@ Client`), and otherwise falls back to the
configured agency name. It never guesses an employer.

#### Scenario: Client resolved from an explicit delimiter

- **WHEN** `Hub` is set and a posting title carries an explicit `— <Client>` suffix
- **THEN** the `Job` company is `<Client>`

#### Scenario: Fallback to the agency name

- **WHEN** `Hub` is set and a posting title exposes no explicit client
- **THEN** the `Job` company is the configured agency name

### Requirement: Per-posting detail isolation

The adapter SHALL fetch posting detail pages under a bounded concurrent worker
pool and drop only the postings whose fetch or parse fails, so one bad posting
never aborts the whole board crawl.

#### Scenario: A failing posting is skipped, not fatal

- **WHEN** one posting's detail page fails to fetch or parse
- **THEN** that posting is dropped and the rest of the board's postings are still
  returned

