## Purpose

The board catalog is the authoritative record of which company is crawled on which ATS,
under what board id, and whether that board is proven to work — replacing the git-tracked
`sources/*.yml` files with a queryable table that carries its own lifecycle.

## ADDED Requirements

### Requirement: A board row is validated before it can exist

The system SHALL validate a board's `provider`, `board`, and `region` before inserting it
into the catalog — whether the insert originates from a recognized crowdsourced
contribution or from a curator's manual addition. A `provider` with no registered adapter
SHALL cause the insert to be rejected rather than stored as a live row. A `board` value
SHALL be required to be non-empty unless the provider declares itself `boardless`, in
which case an empty `board` SHALL be accepted. A row that collides on `(provider,
lower(board), region)` with an existing row whose status is not `retired` SHALL be
rejected as a duplicate.

#### Scenario: Unknown provider is rejected

- **WHEN** a board is submitted for a `provider` with no registered adapter
- **THEN** no `active` or `pending` row is created for it, and the rejection records the
  unknown-provider reason

#### Scenario: Empty board is rejected for a board-based provider

- **WHEN** a board is submitted with an empty `board` for a provider that is not
  `boardless`
- **THEN** the insert is rejected with a reason naming the missing board id

#### Scenario: Empty board is accepted for a boardless provider

- **WHEN** a board is submitted with an empty `board` for a provider declared `boardless`
- **THEN** the insert succeeds

#### Scenario: A duplicate of a live board is rejected

- **WHEN** a board is submitted matching the `(provider, board, region)` of an existing
  row whose status is `pending`, `active`, or `rejected`
- **THEN** the insert is rejected as a duplicate

#### Scenario: A duplicate of a retired board is accepted

- **WHEN** a board is submitted matching the `(provider, board, region)` of an existing
  `retired` row
- **THEN** the insert succeeds as a new row, independent of the retired one

### Requirement: A crowdsourced board starts pending and is proven by its first crawl

A board inserted from a recognized crowdsourced contribution SHALL start at status
`pending` and SHALL be crawled by `cmd/ingest` exactly as an `active` board is. The first
crawl of a `pending` board that completes without a board-level error SHALL transition it
to `active` and record when. A board that fails validation at insert time SHALL be stored
at status `rejected` with a recorded reason, rather than not existing.

#### Scenario: A valid crowdsourced board starts pending

- **WHEN** a recognized contribution names a valid, non-duplicate `(provider, board)`
- **THEN** a row is inserted at status `pending`

#### Scenario: A pending board is crawled like an active one

- **WHEN** `cmd/ingest` runs for a provider with both `pending` and `active` rows
- **THEN** it crawls both, with no distinction in crawl behavior

#### Scenario: First successful crawl activates a pending board

- **WHEN** a `pending` board's crawl completes with no board-level error
- **THEN** its status becomes `active` and the activation time is recorded

#### Scenario: A failing crawl does not activate a pending board

- **WHEN** a `pending` board's crawl fails at the board level
- **THEN** its status remains `pending`

#### Scenario: An invalid submission is recorded as rejected, not discarded

- **WHEN** a recognized contribution fails validation (unknown provider, or a required
  board id is missing)
- **THEN** a row is inserted at status `rejected` carrying the validation failure as its
  reason

### Requirement: A curator adds or retires a board through a dedicated worker

The system SHALL provide `cmd/add-board`, a report-by-default worker that requires
`--apply` to write, for a curator to add a board directly at status `active` (a
curator-verified board has no unproven period) or to retire an existing board by setting
its status to `retired`. Retiring a board SHALL NOT delete its row. `cmd/add-board` SHALL
apply the same validation as a crowdsourced insert.

#### Scenario: Dry run reports without writing

- **WHEN** `cmd/add-board` is run without `--apply`
- **THEN** it reports what it would insert or retire and writes nothing

#### Scenario: Apply inserts an active board

- **WHEN** `cmd/add-board --apply` adds a valid, non-duplicate board
- **THEN** a row is inserted at status `active`

#### Scenario: Apply retires a board without deleting it

- **WHEN** `cmd/add-board --apply` retires an existing board
- **THEN** its row's status becomes `retired` and the row still exists

#### Scenario: An invalid manual addition is rejected the same way

- **WHEN** a curator attempts to add a board with an unknown provider or a missing
  required board id
- **THEN** `cmd/add-board` reports the same validation failure a crowdsourced submission
  would hit, and (under `--apply`) writes nothing
