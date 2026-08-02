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
applied to. When — and only when — the first view row is created for a
`(user, job)` pair, the system SHALL increment that job's materialized
`view_count` by one, in the same statement, so repeat views never inflate it.

#### Scenario: First view by a signed-in user

- **WHEN** an authenticated user sends `POST /api/v1/jobs/:id/view` for a job
  they have not interacted with before
- **THEN** the system creates a `user_jobs` row with `viewed_at` set,
  `saved_at` null, and `applied_at` null
- **AND** responds `200` with
  `{"data": {job_id, viewed_at, saved_at: null, applied_at: null}}`
- **AND** the job's `view_count` is incremented by one

#### Scenario: Repeat view does not duplicate

- **WHEN** an authenticated user views the same job a second time
- **THEN** the existing row's `viewed_at` is refreshed
- **AND** no second row is created
- **AND** the response carries the existing `saved_at` and `applied_at` values
  unchanged
- **AND** the job's `view_count` is not incremented again

#### Scenario: View requires authentication

- **WHEN** a request to `POST /api/v1/jobs/:id/view` carries no valid auth cookie
- **THEN** the system responds `401` and records nothing

#### Scenario: View with a non-numeric id

- **WHEN** an authenticated user sends `POST /api/v1/jobs/:id/view` with an `:id`
  that is not a valid job id
- **THEN** the system responds with a client error (`400`) and records nothing

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

### Requirement: Writing an application by the row the listing served

The system SHALL let an authenticated user set an application's `stage` and/or
`notes` via `PATCH /api/v1/me/applications/:id`, drop it from the board via
`DELETE /api/v1/me/applications/:id`, and clear its progress while keeping its
saved mark via `DELETE /api/v1/me/applications/:id/stage`. The body, the stage
vocabulary, the partial-update rule and the rejection of an unknown stage SHALL be
those the slug-addressed track endpoint already applies.

`:id` SHALL be the row identifier exactly as `GET /api/v1/me/tracking` served it.
That identifier has two forms — an application whose posting the catalogue no
longer holds is named by the application, every other row by its posting's slug —
and the endpoint SHALL accept both, because the interface can only send back what
the listing gave it.

An identifier that names nothing the caller owns SHALL be answered `404` with the
body a missing row produces, whatever its form. "Not an identifier" and "not
yours" MUST be one answer.

The slug-addressed routes SHALL remain registered and unchanged: they are how the
`freehire-cli` and `freehire-mcp` address a posting, and those clients hold no row
identifiers.

#### Scenario: Moving an application whose posting was removed

- **WHEN** the caller sends `PATCH /api/v1/me/applications/:id` with
  `{"stage":"interview"}` for an application the catalogue no longer holds a
  posting for
- **THEN** the stage is recorded and the updated record is returned

#### Scenario: Moving an ordinary application

- **WHEN** the caller sends the same request for a row the listing named by its
  posting's slug
- **THEN** the stage is recorded exactly as the slug-addressed endpoint records it

#### Scenario: Dropping an application from the board

- **WHEN** the caller sends `DELETE /api/v1/me/applications/:id`
- **THEN** the application leaves the board and the row is no longer listed there

#### Scenario: Clearing progress keeps the saved mark

- **WHEN** the caller sends `DELETE /api/v1/me/applications/:id/stage` for a saved
  application
- **THEN** the application leaves the board and the job remains saved

#### Scenario: An identifier that names nothing

- **WHEN** the caller sends any of these requests with an identifier that is
  malformed, or that names a row belonging to somebody else
- **THEN** the system responds `404` with the same body in both cases, and changes
  nothing

#### Scenario: The slug-addressed routes still answer

- **WHEN** a client sends `PATCH /api/v1/jobs/:slug/track`
- **THEN** it behaves exactly as before this change

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

- **WHEN** `GET /api/v1/me/tracking` returns the user's tracked jobs
- **THEN** each row includes the job's `stage` and `notes`

### Requirement: SPA shows and edits application stage and notes

The web SPA's tracking board SHALL, for a signed-in user, show each application's
`stage` as a humanized badge when set, let the user change the stage by dragging
the card between the board's columns or from a control in the opened application
offering the stage vocabulary, and let the user edit `notes` in the opened
application. A signed-out user SHALL see no such controls.

A board card SHALL carry no controls. It is dragged and it is opened, and it
SHALL be draggable from anywhere on it — a card that mounts an interactive
element over its surface cannot be picked up, because the drag library refuses a
gesture that begins on one. Everything the candidate can do to an application
SHALL be offered in the opened application, which has the room for it.

The card SHALL keep its indicators — the stage badge, the silence marker, the
count of linked mail, the mark that it has notes — none of which is a control.

The opened application SHALL offer, beside its stage and notes: a rehearsal, a
follow-up draft, the fit analysis, and CV tailoring. Those that need the posting
SHALL be absent, not disabled, for an application whose posting the catalogue no
longer holds.

#### Scenario: Change a stage by dragging

- **WHEN** a signed-in user drags a card from anywhere on its surface into another
  column
- **THEN** the drag begins, the card lands in that column, and the new stage is
  persisted

#### Scenario: A card carries no controls

- **WHEN** the board renders an application card
- **THEN** the card presents no buttons, and clicking anywhere on it opens the
  application

#### Scenario: Change a stage from the opened application

- **WHEN** a signed-in user selects a new stage in the opened application
- **THEN** the SPA persists it and reflects the new stage on the board

#### Scenario: Edit notes

- **WHEN** a signed-in user edits an application's notes and the field loses focus
- **THEN** the SPA persists the notes

#### Scenario: The actions are offered where there is room

- **WHEN** a signed-in user opens an application whose posting is still listed
- **THEN** the panel offers a rehearsal, the fit analysis and CV tailoring, and
  offers a follow-up draft when the application is owed one

#### Scenario: A posting-less application offers only what it can

- **WHEN** the user opens an application whose posting the catalogue no longer
  holds
- **THEN** the actions needing the posting are absent, and the stage, the notes
  and the follow-up draft still work

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

### Requirement: Tracking routes moved to /me/tracking

The per-user tracking endpoints SHALL be served under `/api/v1/me/tracking` (`""`, `/viewed`, `/pipeline`, `/swipe`, `/analyses`), replacing the previous `/api/v1/me/jobs*` paths, which MUST no longer be registered. This is a breaking API change: clients (the freehire-cli) MUST migrate to `/me/tracking`.

#### Scenario: Canonical tracking path

- **WHEN** a caller requests `GET /api/v1/me/tracking`
- **THEN** it returns the caller's tracked jobs (the listing previously served at `/api/v1/me/jobs`)

#### Scenario: Old path is gone

- **WHEN** a client requests `GET /api/v1/me/jobs`
- **THEN** the system returns `404` — the path is no longer registered

### Requirement: Tracking section renamed with URL redirects

The frontend personal-jobs section SHALL be presented as **Tracking** and served under `/my/tracking/*` (Board, Pipeline, History, AI fit). Requests to the previous `/my/jobs/*` URLs MUST redirect (HTTP 308) to the corresponding `/my/tracking/*` path so existing bookmarks and inbound links keep working.

#### Scenario: Old URL redirects to the new section

- **WHEN** a user opens `/my/jobs/pipeline`
- **THEN** the app redirects to `/my/tracking/pipeline`

#### Scenario: Section labelled Tracking

- **WHEN** a signed-in user opens the tracking section
- **THEN** the navigation and heading read "Tracking", with tabs for Board, Pipeline, History, and AI fit

