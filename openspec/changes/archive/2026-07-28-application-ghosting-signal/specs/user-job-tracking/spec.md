## MODIFIED Requirements

### Requirement: Listing a user's job interactions

The system SHALL expose `GET /api/v1/me/tracking` (auth required) returning the
authenticated user's interactions joined with the public job view shape,
ordered by most recent interaction activity first, with limit/offset
pagination. A `filter` query parameter SHALL narrow the list: `all` (default —
every interaction), `viewed` (view-only rows: neither saved nor applied),
`saved` (`saved_at` set), `applied` (`applied_at` set). The list `meta` SHALL
carry `total/limit/offset` for the active filter plus `counts` with the row
counts of all four filters. Closed jobs SHALL remain in
the listing (their job view carries `closed_at`). An unknown `filter` value
SHALL be a `400`.

Each row that represents an application SHALL additionally carry its
`last_activity_at`, its `days_silent`, and its `silence_state`. A row that is not
an application — no `applied_at` — carries all three as null: a job merely viewed
or saved is not waiting on anyone.

#### Scenario: Listing all interactions

- **WHEN** an authenticated user requests `GET /api/v1/me/tracking`
- **THEN** the response is `200` with
  `{"data": [{job, viewed_at, saved_at, applied_at}, ...], "meta": {...}}`
- **AND** each `job` is the shared job view shape (no internal id)
- **AND** items are ordered by the most recent of the interaction timestamps,
  descending

#### Scenario: Filtering to applications

- **WHEN** the user requests `GET /api/v1/me/tracking?filter=applied`
- **THEN** only interactions with non-null `applied_at` are returned
- **AND** `meta.total` counts only those

#### Scenario: Filtering to viewed-only

- **WHEN** the user requests `GET /api/v1/me/tracking?filter=viewed`
- **THEN** only interactions with null `saved_at` and null `applied_at` are
  returned — the passive view history, without the jobs already acted on

#### Scenario: Tab counts in meta

- **WHEN** the user requests the listing with any filter
- **THEN** `meta.counts` reports `{all, viewed, saved, applied}` for that user

#### Scenario: Closed job stays in the history

- **WHEN** a job the user applied to is later closed
- **THEN** it still appears in the listing and its job view has `closed_at` set

#### Scenario: Unknown filter

- **WHEN** the user requests `GET /api/v1/me/tracking?filter=bogus`
- **THEN** the system responds `400`

#### Scenario: Listing requires authentication

- **WHEN** a request to `GET /api/v1/me/tracking` carries no valid auth cookie
- **THEN** the system responds `401`

#### Scenario: An application carries its silence fields

- **WHEN** the listing returns a row whose `applied_at` is set
- **THEN** that row carries `last_activity_at`, `days_silent` and
  `silence_state`

#### Scenario: A non-application carries none of them

- **WHEN** the listing returns a row the user only viewed or saved
- **THEN** its `last_activity_at`, `days_silent` and `silence_state` are all null
