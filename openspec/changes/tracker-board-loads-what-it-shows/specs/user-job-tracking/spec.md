## MODIFIED Requirements

### Requirement: Listing a user's job interactions

The system SHALL expose `GET /api/v1/me/tracking` (auth required) returning the
authenticated user's interactions joined with a **card projection** of the job,
ordered by most recent interaction activity first, with limit/offset
pagination. A `filter` query parameter SHALL narrow the list: `all` (default —
every interaction), `viewed` (view-only rows: neither saved nor applied),
`saved` (`saved_at` set), `applied` (`applied_at` set). The list `meta` SHALL
carry `total/limit/offset` for the active filter plus `counts` with the row
counts of all four filters. Closed jobs SHALL remain in
the listing (their card carries `closed_at`). An unknown `filter` value
SHALL be a `400`.

The card SHALL carry what a list row draws and no more: `public_slug`, `title`, `company`,
`closed_at`, and the stated facets of its tag row (`work_mode`, `seniority`, `employment_type`,
`countries`, `regions`). It SHALL NOT carry the posting's description. The full public job view
remains available at `GET /api/v1/me/tracking/:slug`, which is the read a caller makes for one
application it has opened.

The listing query SHALL read only the card's columns from `jobs`. Reading the description and
discarding it later would keep the database cost while saving only the transfer.

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
- **AND** each `job` is the card projection (no internal id, no description)
- **AND** items are ordered by the most recent of the interaction timestamps,
  descending

#### Scenario: The listing carries no posting text

- **WHEN** any `GET /api/v1/me/tracking` response is serialized
- **THEN** no row carries the job's description, however large the page

#### Scenario: The full posting is one read away

- **WHEN** the caller opens one application and requests `GET /api/v1/me/tracking/:slug`
- **THEN** the response carries the complete public job view, description included

#### Scenario: Filtering to applications

- **WHEN** the user requests `GET /api/v1/me/tracking?filter=applied`
- **THEN** only interactions with non-null `applied_at` are returned
- **AND** `meta.total` counts only those
