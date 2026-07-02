# user-job-tracking

## Purpose

Give signed-in users a per-job memory: record which jobs they have viewed and
which they have applied to, one row per `(user, job)`. Views are passive history
(recorded silently when a job is opened); applies are explicit (the user confirms
"Yes, I applied"). The SPA surfaces this as an "already applied" badge and a
post-Apply "Did you apply?" prompt. Writes require a session; the public job read
path is untouched. The model is the thin first slice of a personal application
tracker: `applied_at` is the entry point for a future stage pipeline, and the
composite key already guarantees at most one application per `(user, job)`.
## Requirements
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

### Requirement: Marking a job applied

The system SHALL let an authenticated user mark a job as applied, idempotently,
and SHALL seed `stage = 'applied'` when the stage is currently unset (an
already-set stage is left untouched). Authentication MAY be by session cookie or
by API key; either identifies the acting user identically. Marking applied sets
`applied_at`; it works whether or not a view was recorded first, and repeating it
does not create a duplicate or error. The endpoint SHALL return the updated
interaction record.

#### Scenario: Mark applied after viewing

- **WHEN** an authenticated user who has viewed a job sends
  `POST /api/v1/jobs/:id/apply`
- **THEN** the job's `applied_at` is set
- **AND** the response is `200` with `{"data": {job_id, viewed_at, applied_at}}`
  where `applied_at` is non-null

#### Scenario: Mark applied is idempotent

- **WHEN** an authenticated user marks the same job applied twice
- **THEN** the row is updated in place each time
- **AND** no duplicate row is created and no error is returned

#### Scenario: Applying seeds the initial stage

- **WHEN** an authenticated user applies to a job whose `stage` is unset
- **THEN** the interaction's `stage` becomes `applied`
- **AND** applying again, or after the stage has been advanced, leaves the
  existing stage unchanged

#### Scenario: Apply requires authentication

- **WHEN** a request to `POST /api/v1/jobs/:id/apply` carries neither a valid auth
  cookie nor a valid API key
- **THEN** the system responds `401` and records nothing

#### Scenario: Apply authenticated by an API key

- **WHEN** a request to `POST /api/v1/jobs/:id/apply` carries a valid
  `Authorization: Bearer <key>` and no cookie
- **THEN** the system marks the job applied for the key's owning user exactly as a
  cookie session would and responds `200` with the updated interaction record

#### Scenario: Apply to a non-existent job

- **WHEN** an authenticated user sends `POST /api/v1/jobs/:id/apply` with a
  numeric `:id` that has no corresponding job row
- **THEN** the foreign-key violation surfaces as `404`, not `500`

### Requirement: Public job reads are unaffected

The system SHALL keep the public job read path unchanged by this capability.
Reading a job MUST NOT require authentication and MUST NOT record any
interaction.

#### Scenario: Reading a job without a session

- **WHEN** an unauthenticated client sends `GET /api/v1/jobs/:id`
- **THEN** the system responds `200` with the job as before
- **AND** no `user_jobs` row is created

### Requirement: SPA surfaces interaction state on the job view

The web SPA SHALL, for a signed-in user, record a view when a job is opened and
surface the applied state. A job already applied to SHALL show an "applied"
indicator. After the user follows the external apply link, the SPA SHALL offer
an explicit "Did you apply?" choice; confirming marks the job applied, while
declining changes no server state. A signed-out user SHALL see the existing job
view unchanged.

#### Scenario: Opening a job while signed in

- **WHEN** a signed-in user opens a job in the SPA
- **THEN** the SPA records a view for that job
- **AND** if the returned record shows the job was already applied to, the SPA
  shows an "applied" indicator and does not offer the apply prompt

#### Scenario: Confirming an application

- **WHEN** a signed-in user follows the apply link and then confirms "Yes" on the
  "Did you apply?" prompt
- **THEN** the SPA marks the job applied
- **AND** the "applied" indicator appears

#### Scenario: Declining the apply prompt

- **WHEN** a signed-in user chooses "No" on the "Did you apply?" prompt
- **THEN** the prompt is dismissed in the client
- **AND** no application is recorded on the server

#### Scenario: Signed-out user

- **WHEN** a signed-out user opens a job
- **THEN** the job view behaves exactly as before this change
- **AND** no view or apply request is sent

### Requirement: Tracking application stage and notes

The system SHALL let an authenticated user set an application's `stage` and/or
free-text `notes` via `PATCH /api/v1/jobs/:slug/track`, authenticated by session
cookie or API key. The body carries optional `stage` and `notes`, of which at
least one MUST be present (else `400`). The endpoint SHALL upsert the
`(user, job)` interaction (creating it if absent) and apply a partial update — a
field omitted from the body leaves its stored column unchanged. A provided
`stage` MUST be one of the controlled vocabulary values, and an unknown value
SHALL be rejected with `400`. The endpoint SHALL return the updated interaction
record.

The stage vocabulary SHALL be the active stages `applied`, `screening`,
`responded`, `interview`, `offer` and the terminal stages `accepted`,
`rejected`, `withdrawn`. Transitions are unrestricted: any valid stage may be set
from any other.

#### Scenario: Set a stage

- **WHEN** an authenticated user sends `PATCH /api/v1/jobs/:slug/track` with
  `{"stage":"interview"}` for a job they have not interacted with
- **THEN** the system creates the interaction with `stage = interview` and
  responds `200` with the record

#### Scenario: Set notes without changing the stage

- **WHEN** the user sends `{"notes":"recruiter called Friday"}` with no `stage`
- **THEN** `notes` is updated and the existing `stage` is left unchanged

#### Scenario: Unknown stage is rejected

- **WHEN** the user sends `{"stage":"banana"}`
- **THEN** the system responds `400` and changes nothing

#### Scenario: Empty track is rejected

- **WHEN** the user sends `track` with neither `stage` nor `notes`
- **THEN** the system responds `400`

#### Scenario: Track authenticated by an API key

- **WHEN** a `track` request carries a valid `Authorization: Bearer <key>` and no
  cookie
- **THEN** the stage/notes are set for the key's owning user exactly as a cookie
  session would

#### Scenario: Track requires authentication

- **WHEN** a `track` request carries neither a valid cookie nor a valid API key
- **THEN** the system responds `401` and changes nothing

### Requirement: Interaction records carry stage and notes

Interaction records SHALL carry the application's `stage` and `notes` (null when
unset) — on the view, apply, save, unsave, and track responses and on every
my-jobs listing row. No other field of the existing interaction or my-jobs shapes
changes.

#### Scenario: Stage and notes on the interaction response

- **WHEN** any per-user interaction endpoint returns the interaction record
- **THEN** the JSON includes `stage` and `notes` (null when unset) alongside
  `job_id`, `viewed_at`, `saved_at`, `applied_at`

#### Scenario: Stage and notes on the my-jobs listing

- **WHEN** `GET /api/v1/me/jobs` returns the user's tracked jobs
- **THEN** each row includes the job's `stage` and `notes`

### Requirement: SPA shows and edits application stage and notes

The web SPA's My Jobs page SHALL, for a signed-in user, show each tracked job's
`stage` as a humanized badge when set, let the user change the stage from a
control offering the stage vocabulary (persisting via the track endpoint), and
let the user edit `notes` inline (persisting via the track endpoint). A signed-out
user SHALL see no such controls.

#### Scenario: Change a stage

- **WHEN** a signed-in user selects a new stage for a job on My Jobs
- **THEN** the SPA persists it via the track endpoint and reflects the new stage

#### Scenario: Edit notes

- **WHEN** a signed-in user edits a job's notes and the field loses focus
- **THEN** the SPA persists the notes via the track endpoint

### Requirement: Listing the caller's viewed job slugs

The system SHALL let an authenticated caller read the set of public job slugs
they have viewed, so a client can mark already-seen jobs without making the
public job-read path authenticated. Authentication MAY be by session cookie or by
API key; either identifies the acting user identically. The endpoint SHALL return
every `public_slug` for which the caller has a `user_jobs` interaction row,
including closed jobs, as a flat list under `{"data": [...]}`. The public job
list and search endpoints SHALL remain unauthenticated and unchanged.

#### Scenario: Signed-in user reads their viewed slugs

- **WHEN** an authenticated user sends `GET /api/v1/me/jobs/viewed` and has
  previously viewed two jobs
- **THEN** the system responds `200` with `{"data": [slug_a, slug_b]}` containing
  exactly the `public_slug`s of those two jobs

#### Scenario: User with no interactions

- **WHEN** an authenticated user who has viewed no jobs sends
  `GET /api/v1/me/jobs/viewed`
- **THEN** the system responds `200` with `{"data": []}`

#### Scenario: Viewed slugs require authentication

- **WHEN** a request to `GET /api/v1/me/jobs/viewed` carries neither a valid auth
  cookie nor a valid API key
- **THEN** the system responds `401` and returns no slug data

#### Scenario: Viewed slugs are scoped to the caller

- **WHEN** an authenticated user reads their viewed slugs
- **THEN** the response contains only slugs from that user's own `user_jobs`
  rows and never another user's interactions

### Requirement: Dismissing a job

The system SHALL let an authenticated user dismiss a job, idempotently, recording
it as a distinct interaction alongside view/apply/save. Authentication MAY be by
session cookie or by API key; either identifies the acting user identically.
Dismissing sets `user_jobs.dismissed_at`; it works whether or not a view was
recorded first, and repeating it does not create a duplicate or error. Dismissal
is a private triage signal used only to keep a job out of the swipe deck — it
SHALL NOT remove the job from the public `/jobs` list or search. The endpoint
SHALL return the updated interaction record.

#### Scenario: Dismiss a job

- **WHEN** an authenticated user sends `POST /api/v1/jobs/:slug/dismiss`
- **THEN** the job's `dismissed_at` is set
- **AND** the response is `200` with the updated interaction record

#### Scenario: Dismiss is idempotent

- **WHEN** an authenticated user dismisses the same job twice
- **THEN** the row is updated in place each time
- **AND** no duplicate row is created and no error is returned

#### Scenario: Dismiss authenticated by an API key

- **WHEN** a request to `POST /api/v1/jobs/:slug/dismiss` carries a valid
  `Authorization: Bearer <key>` and no cookie
- **THEN** the system dismisses the job for the key's owning user exactly as a
  cookie session would and responds `200`

#### Scenario: Dismiss requires authentication

- **WHEN** a request to `POST /api/v1/jobs/:slug/dismiss` carries neither a valid
  auth cookie nor a valid API key
- **THEN** the system responds `401` and records nothing

#### Scenario: Dismiss does not hide the job elsewhere

- **WHEN** an authenticated user dismisses a job and then loads the `/jobs` list
  or search with filters that match it
- **THEN** the job still appears in the list and search results

### Requirement: Clearing a dismissal

The system SHALL let an authenticated user clear a job's dismissal, idempotently,
so it can re-enter the swipe deck. Clearing SHALL unset `dismissed_at`; clearing
a job that is not currently dismissed SHALL be a no-op that returns success. This
is the undo path for a swipe-left decision.

#### Scenario: Clear a dismissal

- **WHEN** an authenticated user sends `DELETE /api/v1/jobs/:slug/dismiss` for a
  job they previously dismissed
- **THEN** the job's `dismissed_at` is cleared
- **AND** the job is eligible to appear in the swipe deck again

#### Scenario: Clearing a non-dismissed job is a no-op

- **WHEN** an authenticated user sends `DELETE /api/v1/jobs/:slug/dismiss` for a
  job they never dismissed
- **THEN** the system responds successfully and no row is created or errored

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

