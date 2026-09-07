# gr8people-source Specification

## Purpose

Crawls one gr8people/workgr8 employer board (a tenant career-site host) through the platform's
own public GraphQL search API, and recognizes both `*.gr8people.com` and `*.workgr8.com` URLs as
the same provider so a pasted or harvested apply link can be onboarded as a board.

## Requirements

### Requirement: Host-scoped listing crawl

The system SHALL crawl a gr8people board by requesting a fresh anonymous session token from that
board's own careers host, then issuing a GraphQL job search against that host, and SHALL return
one `Job` per posting the response carries.

#### Scenario: Fetch returns the board's postings

- **WHEN** the adapter fetches a board whose host is a live gr8people or workgr8 tenant with open
  postings
- **THEN** it mints a session token from that host and returns one `Job` per posting the host's
  search API answers

#### Scenario: Board with no open postings

- **WHEN** the tenant has zero open postings
- **THEN** the adapter returns an empty result, not an error

### Requirement: Two domains, one provider

The system SHALL treat `*.gr8people.com` and `*.workgr8.com` as the same platform, registered
under one provider key. A board on either domain SHALL be crawled and recognized identically.

#### Scenario: A workgr8 board crawls like a gr8people board

- **WHEN** a board is a `*.workgr8.com` host
- **THEN** the adapter mints its token and issues its search request exactly as it would for a
  `*.gr8people.com` host, under the same provider key

### Requirement: Host-identified board

The board SHALL be the tenant's whole careers host (e.g. `etrade.gr8people.com`), because the
brand domain (`gr8people.com` vs `workgr8.com`) is not derivable from the tenant name alone.

#### Scenario: Well-formed board is accepted

- **WHEN** a board is `"etrade.gr8people.com"`
- **THEN** the adapter mints a token from and queries that exact host

### Requirement: Posting normalization

The adapter SHALL map each returned posting onto the catalogue's `Job` shape: the platform's own
posting key to the external id, the title, the description body (no separate detail request), a
location built from the posting's place(s), a work mode derived from the platform's structured
workplace-type field, the posting's post date, and — when the platform states it — a structured
employment type. The job's company SHALL be the board's configured company.

#### Scenario: A posting maps to a Job

- **WHEN** a posting carries a key, a title, a description body, a place, a workplace type, and a
  post date
- **THEN** the resulting `Job` carries that key as `ExternalID`, the title, the sanitized
  description, the place as `Location`, a `WorkMode` derived from the workplace type, the post
  date as `PostedAt`, and the board's configured company as `Company`

### Requirement: Public URL recognition

The system SHALL recognize a job or listing URL on a `*.gr8people.com` or `*.workgr8.com` host as
a gr8people board URL, resolving it to provider `gr8people` and board `<host>`.

#### Scenario: A gr8people job link resolves to its board

- **WHEN** a URL is `https://etrade.gr8people.com/jobs/4709`
- **THEN** it resolves to provider `gr8people`, board `"etrade.gr8people.com"`

#### Scenario: A workgr8 job link resolves to its board

- **WHEN** a URL is `https://batesville.workgr8.com/jobs/1146`
- **THEN** it resolves to provider `gr8people`, board `"batesville.workgr8.com"`
