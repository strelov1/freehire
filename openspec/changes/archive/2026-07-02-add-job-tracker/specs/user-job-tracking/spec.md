## ADDED Requirements

### Requirement: Saving and unsaving a job

The system SHALL let an authenticated user save (bookmark) a job and unsave it,
keyed by `(user, job)`, idempotently. Saving sets `saved_at` on the interaction
row (creating it if absent); unsaving clears `saved_at` without deleting the
row, so view history and an application mark survive. Both endpoints SHALL
return the resulting interaction record.

#### Scenario: Saving a job

- **WHEN** an authenticated user sends `POST /api/v1/jobs/:slug/save`
- **THEN** the interaction row's `saved_at` is set
- **AND** the response is `200` with `{"data": {job_id, viewed_at, saved_at, applied_at}}`
  where `saved_at` is non-null

#### Scenario: Saving is idempotent

- **WHEN** an authenticated user saves the same job twice
- **THEN** the row is updated in place and no duplicate is created

#### Scenario: Unsaving keeps history

- **WHEN** an authenticated user who has viewed, saved, and applied to a job
  sends `DELETE /api/v1/jobs/:slug/save`
- **THEN** `saved_at` becomes null while `viewed_at` and `applied_at` are
  unchanged

#### Scenario: Unsaving without a prior interaction

- **WHEN** an authenticated user unsaves a job they have no interaction row for
- **THEN** the system responds `200` without creating a row

#### Scenario: Save requires authentication

- **WHEN** a request to `POST /api/v1/jobs/:slug/save` carries no valid auth
  cookie
- **THEN** the system responds `401` and records nothing

#### Scenario: Save on an unknown slug

- **WHEN** an authenticated user saves a slug that resolves to no job
- **THEN** the system responds `404`

### Requirement: Listing a user's job interactions

The system SHALL expose `GET /api/v1/me/jobs` (auth required) returning the
authenticated user's interactions joined with the public job view shape,
ordered by most recent interaction activity first, with limit/offset
pagination. A `filter` query parameter SHALL narrow the list: `all` (default —
every interaction), `viewed` (view-only rows: neither saved nor applied),
`saved` (`saved_at` set), `applied` (`applied_at` set). The list `meta` SHALL
carry `total/limit/offset` for the active filter plus `counts` with the row
counts of all four filters. Closed jobs SHALL remain in
the listing (their job view carries `closed_at`). An unknown `filter` value
SHALL be a `400`.

#### Scenario: Listing all interactions

- **WHEN** an authenticated user requests `GET /api/v1/me/jobs`
- **THEN** the response is `200` with
  `{"data": [{job, viewed_at, saved_at, applied_at}, ...], "meta": {...}}`
- **AND** each `job` is the shared job view shape (no internal id)
- **AND** items are ordered by the most recent of the interaction timestamps,
  descending

#### Scenario: Filtering to applications

- **WHEN** the user requests `GET /api/v1/me/jobs?filter=applied`
- **THEN** only interactions with non-null `applied_at` are returned
- **AND** `meta.total` counts only those

#### Scenario: Filtering to viewed-only

- **WHEN** the user requests `GET /api/v1/me/jobs?filter=viewed`
- **THEN** only interactions with null `saved_at` and null `applied_at` are
  returned — the passive view history, without the jobs already acted on

#### Scenario: Tab counts in meta

- **WHEN** the user requests the listing with any filter
- **THEN** `meta.counts` reports `{all, viewed, saved, applied}` for that user

#### Scenario: Closed job stays in the history

- **WHEN** a job the user applied to is later closed
- **THEN** it still appears in the listing and its job view has `closed_at` set

#### Scenario: Unknown filter

- **WHEN** the user requests `GET /api/v1/me/jobs?filter=bogus`
- **THEN** the system responds `400`

#### Scenario: Listing requires authentication

- **WHEN** a request to `GET /api/v1/me/jobs` carries no valid auth cookie
- **THEN** the system responds `401`

## MODIFIED Requirements

### Requirement: Recording a job view

The system SHALL let an authenticated user record that they viewed a job, keyed
by `(user, job)`, idempotently. The first view creates the interaction; a repeat
view refreshes its timestamp without creating a duplicate. The endpoint SHALL
return the interaction record, including whether the job has been saved and
applied to.

#### Scenario: First view by a signed-in user

- **WHEN** an authenticated user sends `POST /api/v1/jobs/:id/view` for a job
  they have not interacted with before
- **THEN** the system creates a `user_jobs` row with `viewed_at` set,
  `saved_at` null, and `applied_at` null
- **AND** responds `200` with
  `{"data": {job_id, viewed_at, saved_at: null, applied_at: null}}`

#### Scenario: Repeat view does not duplicate

- **WHEN** an authenticated user views the same job a second time
- **THEN** the existing row's `viewed_at` is refreshed
- **AND** no second row is created
- **AND** the response carries the existing `saved_at` and `applied_at` values
  unchanged

#### Scenario: View requires authentication

- **WHEN** a request to `POST /api/v1/jobs/:id/view` carries no valid auth cookie
- **THEN** the system responds `401` and records nothing

#### Scenario: View with a non-numeric id

- **WHEN** an authenticated user sends `POST /api/v1/jobs/:id/view` with an `:id`
  that is not a valid job id
- **THEN** the system responds with a client error (`400`) and records nothing
