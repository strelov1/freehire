## MODIFIED Requirements

### Requirement: Marking a job applied

The system SHALL let an authenticated user mark a job as applied, idempotently, and SHALL seed
`stage = 'applied'` when the stage is currently unset (an already-set stage is left untouched).
Authentication MAY be by session cookie or by API key; either identifies the acting user
identically. Marking applied sets `applied_at`; it works whether or not a view was recorded
first, and repeating it does not create a duplicate or error. The endpoint SHALL return the
updated interaction record. When — and only when — `applied_at` transitions from unset to set
for a `(user, job)` pair, the system SHALL increment that job's materialized `applied_count` by
one, in the same statement, so repeat applies never inflate it.

The request MAY carry a body naming the day the application was sent, as a calendar date
(`YYYY-MM-DD`). The system SHALL record that day rather than the current time, and SHALL store
it as noon UTC: the caller is stating a day, and midnight renders as the previous date for every
reader west of Greenwich. A request with no body, or with no date in its body, SHALL behave
exactly as before.

A stated date SHALL be refused with `400` when it is in the future or more than a year in the
past, using the same bounds the ghost report applies, so the system holds one answer to which
dates are believable rather than two.

A stated date SHALL override a date already recorded for that application, because the person
naming it knows better than any value the system derived. This is the opposite of the rule for
an application reconstructed from employer mail, whose date is an upper bound read off a
message and MUST NOT overwrite one the candidate asserted.

Re-dating an application SHALL NOT change `applied_count`: correcting when an application
happened is not a second application.

#### Scenario: Mark applied after viewing

- **WHEN** an authenticated user who has viewed a job sends `POST /api/v1/jobs/:id/apply`
- **THEN** the job's `applied_at` is set
- **AND** the response is `200` with `{"data": {job_id, viewed_at, applied_at}}` where
  `applied_at` is non-null
- **AND** the job's `applied_count` is incremented by one

#### Scenario: Mark applied is idempotent

- **WHEN** an authenticated user marks the same job applied twice
- **THEN** the row is updated in place each time
- **AND** no duplicate row is created and no error is returned
- **AND** the job's `applied_count` is incremented only on the first apply, not the second

#### Scenario: Applying seeds the initial stage

- **WHEN** an authenticated user applies to a job whose `stage` is unset
- **THEN** the interaction's `stage` becomes `applied`
- **AND** applying again, or after the stage has been advanced, leaves the existing stage
  unchanged

#### Scenario: Applying on a stated day

- **WHEN** an authenticated user sends `POST /api/v1/jobs/:slug/apply` with body
  `{"applied_on": "2026-07-27"}` for a job they have not applied to
- **THEN** the application's `applied_at` is 27 July 2026 at noon UTC, not the current time
- **AND** the response reports that date

#### Scenario: A stated day corrects a date already recorded

- **WHEN** an authenticated user applies with a stated day to a job already marked applied
- **THEN** the application's `applied_at` becomes the stated day
- **AND** the job's `applied_count` is unchanged

#### Scenario: An unbelievable date is refused

- **WHEN** a request states a day in the future, or more than a year in the past
- **THEN** the system responds `400` and records nothing

#### Scenario: A malformed date is refused

- **WHEN** a request states `applied_on` that is not a `YYYY-MM-DD` calendar date
- **THEN** the system responds `400` and records nothing

#### Scenario: Apply requires authentication

- **WHEN** a request to `POST /api/v1/jobs/:id/apply` carries neither a valid auth cookie nor a
  valid API key
- **THEN** the system responds `401` and records nothing

#### Scenario: Apply authenticated by an API key

- **WHEN** a request to `POST /api/v1/jobs/:id/apply` carries a valid `Authorization: Bearer
  <key>` and no cookie
- **THEN** the system marks the job applied for the key's owning user exactly as a cookie
  session would and responds `200` with the updated interaction record

#### Scenario: Apply to a non-existent job

- **WHEN** an authenticated user sends `POST /api/v1/jobs/:id/apply` with a numeric `:id` that
  has no corresponding job row
- **THEN** the foreign-key violation surfaces as `404`, not `500`
