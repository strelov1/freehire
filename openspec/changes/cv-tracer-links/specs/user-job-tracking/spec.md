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

An application row SHALL also carry `cv_opened_at`: when a CV of the caller's that is tied to
this job was last opened by a non-automated visitor, and null when the caller has no such CV or
it has never been opened. This field SHALL NOT be an input to `last_activity_at`,
`days_silent` or `silence_state` — a CV being opened is not a reply, and folding it into the
silence derivation would clear the marker at the moment it matters most.

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

#### Scenario: An application whose traced CV was opened

- **WHEN** the listing returns an application row and a CV of the caller's tied to that job was
  opened by a non-automated visitor
- **THEN** that row's `cv_opened_at` is the most recent such open

#### Scenario: Opening a CV leaves the silence fields alone

- **WHEN** a click is recorded against a CV tied to an application and the listing is read
- **THEN** that row's `cv_opened_at` is set
- **AND** its `last_activity_at`, `days_silent` and `silence_state` are what they were before
  the click

#### Scenario: An application with no traced CV

- **WHEN** the listing returns an application row with no traced CV for that job
- **THEN** its `cv_opened_at` is null
