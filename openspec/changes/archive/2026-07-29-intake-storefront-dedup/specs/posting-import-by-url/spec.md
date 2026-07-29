## MODIFIED Requirements

### Requirement: A parseable page is imported into the catalog

The system SHALL resolve a URL the catalog does not carry through the link-source
registry — the host-scoped adapters first, the generic `JobPosting` resolver last — and,
when a single vacancy is parsed, write it under the destination's own
`(source, external_id)` through the canonical job write path, enqueuing it for enrichment.
The response SHALL carry the resulting `public_slug` and status `imported`.

This applies only when the catalog holds no posting of the same vacancy. When it does, the
collapse behaviour below governs instead, and neither the enrichment enqueue nor the
`imported` status applies.

#### Scenario: A vacancy page is imported and answered with its slug

- **WHEN** an authenticated user submits the URL of a vacancy the catalog lacks and an
  adapter parses it
- **THEN** the response is 201 with the new posting's `public_slug` and status `imported`,
  and the posting is readable from the catalog by that slug

#### Scenario: A re-submitted import resolves to the same posting

- **WHEN** the same URL is submitted twice
- **THEN** the second call answers with the same `public_slug` and creates no second
  posting

## ADDED Requirements

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
