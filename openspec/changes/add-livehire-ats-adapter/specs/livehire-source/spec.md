## Purpose

Crawls LiveHire-hosted career sites (<https://www.livehire.com/careers/><segment>) into the job catalogue
via LiveHire's public `/careers-api` JSON API — segment-scoped listing, pagination, per-posting
detail hydration.

## ADDED Requirements

### Requirement: Board-scoped crawl

The system SHALL provide a `livehire` source adapter that crawls one LiveHire-hosted tenant's
public career site into the catalogue. The board id is the tenant's career-site segment (the
path segment in `https://www.livehire.com/careers/<segment>`, e.g. `flybuys`, which is also the
`segmentCode` the API calls carry). The adapter SHALL return one `Job` per open posting the
tenant currently lists.

#### Scenario: A board yields all its live postings

- **WHEN** the adapter crawls a configured LiveHire board whose tenant currently has several
  live postings
- **THEN** it returns one `Job` per posting

#### Scenario: A tenant with no postings yields an empty crawl

- **WHEN** a configured board's segment exists but currently lists no postings
- **THEN** the adapter completes the crawl with no postings rather than failing it

### Requirement: Listing is walked to completion without duplicates

The listing endpoint reports `hasMoreResults` and `totalCount`; the adapter SHALL follow
pagination while `hasMoreResults` holds, SHALL dedup postings across pages by external id (the
endpoint has been observed re-serving the full set on every page), and SHALL stop when the set
stops growing.

#### Scenario: A re-serving page does not duplicate postings

- **WHEN** a later listing page returns postings already gathered from an earlier page
- **THEN** each posting appears exactly once in the crawl's results and the walk ends

### Requirement: Internal and unusable postings are excluded

The adapter SHALL exclude a posting the listing marks `internalOnly` and a posting missing the
identifiers (`urlCode`, `advertUrlCode`) its detail and URL are addressed by.

#### Scenario: An internal-only posting is never returned

- **WHEN** a board's listing includes a posting flagged `internalOnly`
- **THEN** the adapter does not return a `Job` for that posting

### Requirement: Per-posting hydration

For each surviving posting not already ingested, the adapter SHALL fetch that posting's own
detail and map: the platform's `<segment>_<urlCode>_<advertUrlCode>` identity to `ExternalID`;
the constructed `https://www.livehire.com/careers/<segment>/job/<urlCode>/<advertUrlCode>/<slug>`
address to the posting URL; the role name; the HTML body to a sanitized description; the stated
work arrangement (`flexibleWorkOption`) to the work-mode field; the stated country to the
posting's country field; and the stated employment type to the controlled vocabulary. A posting
already ingested SHALL be re-listed as a liveness refresh carrying only identity, without a
detail request.

#### Scenario: A posting with full detail maps completely

- **WHEN** a posting's detail fetch succeeds and states a description, work arrangement, and
  location
- **THEN** the resulting `Job` carries all of them

#### Scenario: A failed detail fetch falls back to list-only

- **WHEN** one posting's detail fetch fails while the rest of the board's postings fetch
  successfully
- **THEN** that posting is still returned with its listing-stage fields (no description) and
  every other posting on the board is returned with full detail

#### Scenario: A seen posting costs no detail request

- **WHEN** a listing returns a posting the catalogue already has
- **THEN** the adapter does not fetch its detail and marks the yielded job as a seen-refresh

### Requirement: Salary published only from structured bounds

The adapter SHALL set a posting's salary range only when the posting's detail states both bounds
and a currency in its own structured fields, and SHALL leave the range unset otherwise.

#### Scenario: A bounded, currency-stated salary is published

- **WHEN** a posting's detail states a minimum and maximum salary and a currency
- **THEN** the resulting `Job` carries that range and its currency

#### Scenario: A missing bound leaves the salary unset

- **WHEN** a posting's detail states no bounds (or a currency-less range string only)
- **THEN** the resulting `Job` carries no salary range

### Requirement: Closed postings are closed by identity

The adapter SHALL mark as removed a posting whose detail reports applications closed or a close
timestamp, so the pipeline closes the catalogue row by identity rather than waiting for the
unseen-job sweep.

#### Scenario: A posting whose applications closed is marked removed

- **WHEN** a posting's detail reports its applications closed
- **THEN** the adapter yields that posting marked removed instead of as a live posting

### Requirement: Stable dedup identity

The adapter SHALL set each `Job`'s `ExternalID` to `<segment>_<urlCode>_<advertUrlCode>`, so
re-crawling the same board dedups to the same catalogue row across runs.

#### Scenario: Re-crawl dedups to one row

- **WHEN** the same posting is crawled on two separate runs
- **THEN** both map to the same `ExternalID` and upsert to a single catalogue row
