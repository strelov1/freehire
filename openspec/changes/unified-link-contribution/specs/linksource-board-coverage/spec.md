## ADDED Requirements

### Requirement: Single-vacancy resolution on any recognised ATS board

The system SHALL resolve a single vacancy from a job link whose host belongs to a
recognised multi-tenant ATS, by deriving `(source, board)` from the URL, fetching that
tenant's board through the ingest adapter already registered for that provider, and
selecting the posting the link identifies. This SHALL apply to every host the board
recogniser knows, without a bespoke single-page adapter per platform.

#### Scenario: A vacancy on a board with no single-page adapter is resolved

- **WHEN** a link is submitted for a vacancy on a recognised ATS board for which no
  host-scoped link-source adapter exists, and an ingest adapter is registered for that
  provider
- **THEN** the vacancy is resolved and carries the same `(source, external_id)` identity
  the ingest crawl of that board would produce

#### Scenario: Coverage follows the board recogniser

- **WHEN** a host is added to the recognised-ATS table and its provider has an ingest
  adapter
- **THEN** links on that host resolve through this path with no further adapter code

### Requirement: Board-coverage resolution runs after host-scoped adapters

The system SHALL attempt board-coverage resolution only after every host-scoped
link-source adapter has declined the link, so a platform with a dedicated adapter keeps
using its cheaper per-vacancy API.

#### Scenario: A Greenhouse link uses the dedicated adapter

- **WHEN** a Greenhouse vacancy link is submitted
- **THEN** it is resolved by the Greenhouse adapter's per-job API and no whole board is
  fetched

### Requirement: A board that does not contain the linked vacancy resolves nothing

The system SHALL report that no vacancy was resolved — not an error — when the tenant's
board is fetched successfully but contains no posting matching the submitted link. A
failure to fetch the board SHALL be reported as an error so the caller can retry or fall
back.

#### Scenario: A closed vacancy is not resolved

- **WHEN** a link points at a posting the board no longer lists
- **THEN** nothing is resolved, no error is raised, and the caller falls back to recording
  the link

#### Scenario: An unreachable board is an error

- **WHEN** the tenant's board cannot be fetched
- **THEN** the caller is given an error rather than a "nothing found" answer

### Requirement: The board recogniser is one shared definition

The system SHALL expose URL-to-`(source, board)` recognition as a single definition used
by both the contribution flow and link resolution, so a host added once is recognised by
both.

#### Scenario: One host entry serves both flows

- **WHEN** a new ATS host is added to the recogniser
- **THEN** both the contribution flow and link resolution recognise links on that host
