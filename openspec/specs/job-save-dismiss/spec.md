# job-save-dismiss Specification

## Purpose
Saving, dismissing and listing the caller's viewed and analysed jobs. Split out of user-job-tracking.

## Requirements

### Requirement: Listing the caller's viewed job slugs

The system SHALL let an authenticated caller read the set of public job slugs
they have viewed, so a client can mark already-seen jobs without making the
public job-read path authenticated. Authentication MAY be by session cookie or by
API key; either identifies the acting user identically. The endpoint SHALL return
every `public_slug` for which the caller has a `user_jobs` interaction row,
including closed jobs, as a flat list under `{"data": [...]}`. The public job
list and search endpoints SHALL remain unauthenticated and unchanged.

#### Scenario: Signed-in user reads their viewed slugs

- **WHEN** an authenticated user sends `GET /api/v1/me/tracking/viewed` and has
  previously viewed two jobs
- **THEN** the system responds `200` with `{"data": [slug_a, slug_b]}` containing
  exactly the `public_slug`s of those two jobs

#### Scenario: User with no interactions

- **WHEN** an authenticated user who has viewed no jobs sends
  `GET /api/v1/me/tracking/viewed`
- **THEN** the system responds `200` with `{"data": []}`

#### Scenario: Viewed slugs require authentication

- **WHEN** a request to `GET /api/v1/me/tracking/viewed` carries neither a valid auth
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

### Requirement: Analysed-jobs list endpoint

The system SHALL provide an authenticated `GET /api/v1/me/tracking/analyses` endpoint that lists the jobs the caller has run the AI fit analysis on, newest first, without invoking the LLM. Each item MUST carry the job's public slug, title, company, a `closed` flag, the analysis `overall_score` and `verdict`, the analysed timestamp, and a `stale` flag (true when the caller's CV, the job content, or the model changed since the analysis was computed). The response MUST include the caller's fit-analysis `quota` (used/limit/remaining) in `meta`. The endpoint accepts a session cookie or an API key.

#### Scenario: List returns analysed jobs with quota

- **WHEN** a signed-in caller who has analysed two jobs requests `GET /api/v1/me/tracking/analyses`
- **THEN** the response is `{ "data": [<two items newest first>], "meta": { "quota": { "used": 2, "limit": 10, "remaining": 8 } } }`, each item carrying slug/title/company/closed/overall_score/verdict/analysed-at/stale, and no LLM call is made

#### Scenario: Closed analysed job is retained with a flag

- **WHEN** the caller analysed a job that has since closed
- **THEN** it still appears in the list with `closed: true`

#### Scenario: Stale analysis is flagged

- **WHEN** the caller's CV was re-uploaded after an analysis was computed
- **THEN** that item is returned with `stale: true`
