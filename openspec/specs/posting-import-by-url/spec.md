# posting-import-by-url Specification

## Purpose

TBD - created by archiving change import-posting-by-url. Update Purpose after archive.

## Requirements

### Requirement: A page already in the catalog is answered without importing

The system SHALL, before any outbound fetch, resolve the requested URL against the catalog
by source identity and then by stored posting URL. When either resolves, the system SHALL
answer with that posting's `public_slug` and status `found`, and SHALL NOT fetch the page
or write any row.

#### Scenario: A carried posting is returned as found

- **WHEN** an authenticated user submits the URL of a page whose posting the catalog
  already carries
- **THEN** the response is 200 with that posting's `public_slug` and status `found`, and no
  new posting is created

### Requirement: A parseable page is imported into the catalog

The system SHALL resolve a URL the catalog does not carry through the link-source
registry — the host-scoped adapters first, then board-coverage resolution, and the generic
`JobPosting` resolver last — and, when a single vacancy is parsed, write it under the
destination's own `(source, external_id)` through the canonical job write path, enqueuing
it for enrichment. The response SHALL carry the resulting `public_slug` and a status
distinguishing whether the board behind it is already crawled (`tracked`) or newly
discovered (`imported`).

#### Scenario: A vacancy page is imported and answered with its slug

- **WHEN** an authenticated user submits the URL of a vacancy the catalog lacks and an
  adapter parses it
- **THEN** the response is 201 with the new posting's `public_slug`, and the posting is
  readable from the catalog by that slug

#### Scenario: A re-submitted import resolves to the same posting

- **WHEN** the same URL is submitted twice
- **THEN** the second call answers with the same `public_slug` and creates no second
  posting

#### Scenario: A vacancy on an already-crawled board is imported as tracked

- **WHEN** an authenticated user submits a vacancy URL whose board the catalog already
  crawls, but whose posting has not yet landed
- **THEN** the vacancy is imported immediately, the response status is `tracked`, and the
  answer identifies the company already being crawled

### Requirement: An unparseable page is queued for triage

The system SHALL, when no adapter parses the page, record the link through the
contribution service rather than discarding it, and answer with status `queued` and a null
`public_slug`. A link that is not an `http(s)` URL SHALL be rejected as unprocessable
instead of recorded.

#### Scenario: A page with no vacancy markup is queued

- **WHEN** an authenticated user submits an `http(s)` URL that no adapter can parse into a
  vacancy
- **THEN** the response is 202 with status `queued` and a null `public_slug`, and the link
  is recorded for manual triage

#### Scenario: A non-URL is rejected

- **WHEN** the submitted value is not an `http(s)` URL
- **THEN** the response is 422 and nothing is recorded

### Requirement: Import requests are authenticated and rate limited

The system SHALL require authentication for this endpoint and SHALL count its calls
against the same per-user budget as board contributions, because both make the server
fetch a user-supplied URL. Outbound fetches SHALL use the SSRF-guarded HTTP client.

#### Scenario: An unauthenticated import is refused

- **WHEN** the endpoint is called without credentials
- **THEN** the response is 401 and no fetch is made

#### Scenario: Exceeding the shared budget is refused

- **WHEN** a user's combined import and board-contribution calls exceed the hourly budget
- **THEN** further calls are refused with 429

### Requirement: A vacancy the catalog already carries is collapsed onto it

The system SHALL, before writing a posting under the generic URL-keyed identity, look for an
open canonical posting of the same role cluster — the same `company_slug` and
`role_fingerprint`, excluding the row being written by its own `(source, external_id)`. When
one exists, the system SHALL still write the row, SHALL mark it a duplicate of that posting,
SHALL NOT enqueue it for enrichment, and SHALL NOT make it searchable.

The row is written rather than skipped because it is what makes the submitted URL resolvable:
URL resolution answers a duplicate with the posting it duplicates, so the link reaches the
canonical card.

A posting written under a board's own identity is not subject to this check — such a posting
is already deduplicated by its `(source, external_id)` uniqueness.

A failed lookup SHALL NOT block the import: the posting is written unmarked.

#### Scenario: A storefront link collapses onto the crawled posting

- **WHEN** an authenticated user submits a vacancy URL on a company's own careers domain, and
  the catalog already carries that vacancy from a crawled source
- **THEN** no second searchable posting appears, the written row is marked a duplicate of the
  carried posting, and no enrichment is queued for it

#### Scenario: The submitted URL resolves to the canonical posting

- **WHEN** that same storefront URL is resolved afterwards
- **THEN** the answer is the canonical posting's `public_slug`, not the collapsed row's

#### Scenario: An unrelated vacancy is imported normally

- **WHEN** the submitted vacancy shares no role cluster with any open posting
- **THEN** it is imported as its own posting and queued for enrichment, as before

### Requirement: A collapsed vacancy is answered found, with its board still recorded

The system SHALL answer a link whose vacancy was collapsed onto an existing posting with
status `found` and the canonical `public_slug`, and SHALL record the contribution for the
board behind that link before answering.

Recording first is required because the board fronting a storefront may be one the system does
not yet crawl: the vacancy being known says nothing about the board being known.

#### Scenario: The answer names the posting the catalog already had

- **WHEN** an authenticated user submits a storefront link whose vacancy the catalog carries
- **THEN** the response is 200 with status `found` and the canonical posting's `public_slug`

#### Scenario: The board is still queued for onboarding

- **WHEN** that link's board is one the system does not crawl
- **THEN** a contribution row for it exists after the call

### Requirement: A successful import still records the board for onboarding

The system SHALL record a contribution for the board behind an imported vacancy whenever
that board is not already crawled, so importing one posting does not hide the remaining
postings on the same board from the onboarding queue. Recording SHALL NOT change the
answer given to the caller, and a failure to record SHALL NOT fail the import.

#### Scenario: Importing one vacancy queues its board

- **WHEN** a vacancy is imported from a board the catalog does not crawl
- **THEN** a contribution for that board is recorded and the caller still receives the
  imported posting's slug

#### Scenario: An already-crawled board is not queued again

- **WHEN** a vacancy is imported from a board the catalog already crawls
- **THEN** no contribution is recorded, because the board needs no onboarding

### Requirement: Every intake is attributed to a user and a surface

The system SHALL attribute every intake request to the authenticated user who made it and
to the surface it arrived from — the website, the Telegram bot, the browser extension, or
the CLI — and SHALL persist that attribution for every outcome, so repeated or abusive use
is visible in stored data rather than only in logs.

#### Scenario: An import records who asked for it

- **WHEN** an authenticated user imports a vacancy
- **THEN** the stored record identifies that user and the surface the request came from

#### Scenario: An unrecognised surface is recorded as unknown rather than refused

- **WHEN** a request carries no surface tag or one the server does not know
- **THEN** the request is served normally and the attribution records an unknown surface

### Requirement: One intake sequence serves every surface

The system SHALL apply the same ordered sequence — catalog lookup, then import, then
contribution recording — to links arriving from the website, the Telegram bot, the browser
extension, and the CLI, so the outcome depends on the link and not on where it was pasted.

#### Scenario: The same link yields the same outcome from any surface

- **WHEN** the same vacancy URL is submitted from the website and from the Telegram bot
- **THEN** both receive the same outcome and the same posting slug when one is imported

#### Scenario: A Telegram user is given the imported posting

- **WHEN** a Telegram user sends a link to a vacancy that can be imported
- **THEN** the bot replies with a link to the posting rather than only acknowledging the
  board
