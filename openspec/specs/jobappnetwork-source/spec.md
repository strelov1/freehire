# jobappnetwork-source Specification

## Purpose

Crawls one talentReef/jobappnetwork employer board (a numeric client id) through the platform's
public listing API, admitting only postings that are actually open to external applicants, and
recognizes the platform's public URL shape so a pasted or harvested apply link can be onboarded as
a board.

## Requirements

### Requirement: Client-scoped listing crawl

The system SHALL crawl a jobappnetwork board by issuing a listing request scoped to that board's
numeric client id, and SHALL return one `Job` per posting the response carries for that client.
The adapter SHALL be registered under provider key `jobappnetwork` in the source registry.

#### Scenario: Fetch returns only the configured client's postings

- **WHEN** the adapter fetches a board whose client id keys an employer with open postings
- **THEN** it issues a listing request scoped to that client id and returns one `Job` per posting
  in the response, and no posting belonging to a different client id

#### Scenario: Client with no open postings

- **WHEN** the client id has zero open postings
- **THEN** the adapter returns an empty result, not an error

### Requirement: External-only visibility

The system SHALL exclude a posting that the platform marks as visible only to internal
(non-public) applicants. Such a posting SHALL NOT be returned as a `Job`.

#### Scenario: Internal-only posting is excluded

- **WHEN** a client's postings include one marked internal-only alongside others open to the
  public
- **THEN** the adapter returns a `Job` for the public ones and none for the internal-only one

### Requirement: Numeric board identifier

The board SHALL be the client's numeric id, exactly as it appears in the platform's own public
posting URLs. A board that is not a positive integer SHALL be rejected before any request is
issued.

#### Scenario: Well-formed board is accepted

- **WHEN** a board is `"20448"`
- **THEN** the adapter scopes its listing request to client id 20448

#### Scenario: Malformed board is rejected

- **WHEN** a board is empty, non-numeric, or zero/negative
- **THEN** the adapter returns an error identifying the expected numeric client id, and issues no
  request

### Requirement: Posting normalization

The adapter SHALL map each returned posting onto the catalogue's `Job` shape: the platform's own
posting id to the external id, the title, the already-formed description body (no separate detail
request), a structured location built from the posting's city/state/country, the country code for
`Countries`, and the posting's creation date for `PostedAt`. The job's company SHALL be the
board's configured company, not a name read off the posting.

#### Scenario: A posting maps to a Job

- **WHEN** a posting carries a numeric posting id, a title, a description body, and a city/state/
  country address
- **THEN** the resulting `Job` carries that id as `ExternalID`, the title, the sanitized
  description, a "City, State" location, the country code, the creation date as `PostedAt`, and
  the board's configured company as `Company`

### Requirement: Public URL recognition

The system SHALL recognize `apply.jobappnetwork.com/clients/<clientId>/posting/<postingId>/…` as
a jobappnetwork board URL, resolving it to provider `jobappnetwork` and board `<clientId>`, so the
board-contribution and link-resolution flows can onboard it without a person having to state the
provider by hand.

#### Scenario: An apply link resolves to its board

- **WHEN** a URL is `https://apply.jobappnetwork.com/clients/20448/posting/7216553/`
- **THEN** it resolves to provider `jobappnetwork`, board `"20448"`

#### Scenario: A URL with no client segment is not recognized

- **WHEN** a URL on the host carries no `/clients/<id>/…` path (for example the bare marketing
  root)
- **THEN** it is not recognized as a board — no false board is produced
