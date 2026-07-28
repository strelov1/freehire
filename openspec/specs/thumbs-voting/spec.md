# thumbs-voting Specification

## Purpose
TBD - created by archiving change add-thumbs-voting. Update Purpose after archive.
## Requirements
### Requirement: A signed-in user holds at most one vote per target

The system SHALL let an authenticated user cast a thumbs **up** or thumbs
**down** on a job and on a company, storing at most one vote per (user, target).
A job vote SHALL be stored on the caller's existing `user_jobs` row as a `vote`
column constrained to the values `-1` (down), `1` (up), or absent (no vote). A
company vote SHALL be stored in a `company_votes` table keyed by
`(user_id, company_slug)`. Casting is idempotent: the same request repeated
leaves the same single vote.

#### Scenario: First up-vote on a job is stored

- **WHEN** a signed-in user casts an up-vote on a job they have not voted on
- **THEN** their `user_jobs` row for that job has `vote = 1`
- **AND** no second vote row is created

#### Scenario: First down-vote on a company is stored

- **WHEN** a signed-in user casts a down-vote on a company they have not voted on
- **THEN** a `company_votes` row exists for `(user, company)` with `vote = -1`

#### Scenario: Re-casting the same direction clears the vote

- **WHEN** a user who has already up-voted a target casts an up-vote again
- **THEN** their vote on that target is removed (no up and no down)

#### Scenario: Casting the opposite direction flips the vote

- **WHEN** a user who has up-voted a target casts a down-vote
- **THEN** their stored vote for that target becomes `-1` (down), replacing the up-vote

#### Scenario: Vote requires authentication

- **WHEN** a request with no session cookie and no API key attempts to cast a vote
- **THEN** the system responds `401` and stores nothing

### Requirement: Each job and company carries materialized public vote counters

Each job and each company SHALL carry two non-negative integer counters,
`upvote_count` and `downvote_count`, materialized on its own row (default `0`).
They count distinct signed-in users whose current stored vote on that target is
up (`1`) or down (`-1`) respectively. Every counter-changing vote write SHALL
update the affected target's counters within the same database transaction so
they never drift from the underlying votes. Read paths SHALL serve both values
directly from the target row without any per-request counting or join.

#### Scenario: Counters default to zero

- **WHEN** a job or company has no votes
- **THEN** its `upvote_count` and `downvote_count` are both `0`

#### Scenario: Counters move with the vote

- **WHEN** a user up-votes a job whose `upvote_count` was 4
- **THEN** the job's `upvote_count` becomes 5 in the same transaction as the vote

#### Scenario: Flipping a vote adjusts both counters

- **WHEN** a user who up-voted a company (counters up=3, down=1) flips to a down-vote
- **THEN** the company's counters become up=2, down=2

#### Scenario: Clearing a vote decrements its counter

- **WHEN** a user clears their up-vote on a job whose `upvote_count` was 5
- **THEN** the job's `upvote_count` becomes 4

#### Scenario: Existing rows are backfilled on release

- **WHEN** the change is released against a database that already holds `user_jobs`
  rows (none of which carry a vote yet)
- **THEN** every job's and company's `upvote_count` and `downvote_count` start at `0`

### Requirement: Authenticated endpoints cast and clear a vote

The system SHALL expose authenticated endpoints to set and clear a vote on a job
and on a company, addressed by the public slug and guarded by the same
`RequireAuthOrKey` used by the other per-user job interactions. `POST
/api/v1/jobs/:slug/vote` and `POST /api/v1/companies/:slug/vote` SHALL accept a
body selecting the up or down direction and apply the toggle/flip semantics.
`DELETE /api/v1/jobs/:slug/vote` and `DELETE /api/v1/companies/:slug/vote` SHALL
remove the caller's vote if present and be a no-op otherwise. Each response
SHALL return the target's resulting `upvote_count`, `downvote_count`, and the
caller's `my_vote`.

#### Scenario: POST up-vote returns updated counters

- **WHEN** a signed-in user `POST`s an up direction to `/api/v1/jobs/:slug/vote`
- **THEN** the response `data` includes the job's new `upvote_count`,
  `downvote_count`, and `my_vote = 1`

#### Scenario: DELETE clears the caller's vote

- **WHEN** a user who has voted `DELETE`s `/api/v1/companies/:slug/vote`
- **THEN** their vote is removed and the response reports `my_vote = 0` with the
  decremented counter

#### Scenario: DELETE with no existing vote is a no-op

- **WHEN** a user with no vote on a job `DELETE`s its vote endpoint
- **THEN** the response is `200`, counters are unchanged, and `my_vote = 0`

#### Scenario: Invalid direction is rejected

- **WHEN** a `PUT` body carries a direction that is neither up nor down
- **THEN** the system responds `400` before any database write

#### Scenario: Unknown slug returns not found

- **WHEN** a vote is cast on a slug that resolves to no job or company
- **THEN** the system responds `404`

### Requirement: Public wire shapes expose the counters and the caller vote

The public job wire shape (`jobview`) and the company response shape SHALL expose
`upvote_count` and `downvote_count` as integer fields on every read (list,
detail, search for jobs; detail for companies), populated from the target row.
When the request is authenticated, the shape SHALL additionally carry `my_vote`
as `-1`, `0`, or `1` reflecting the caller's stored vote; for anonymous reads
`my_vote` SHALL be `0`.

#### Scenario: Job detail exposes counters

- **WHEN** a client requests `GET /api/v1/jobs/:slug`
- **THEN** the `data` object includes integer `upvote_count` and `downvote_count`

#### Scenario: Authenticated read reports the caller's vote

- **WHEN** a signed-in user who down-voted a company requests that company
- **THEN** the response carries `my_vote = -1`

#### Scenario: Anonymous read reports no vote

- **WHEN** an anonymous client reads a job
- **THEN** the response carries `my_vote = 0` and still includes the public counters

### Requirement: The SPA renders a thumbs vote control

The job detail page and the company page SHALL render a thumbs up / thumbs down
control that shows the two public counters and highlights the caller's own vote.
Activating a direction SHALL call the vote endpoint and reflect the returned
counters and `my_vote` optimistically. For an anonymous visitor the control SHALL
prompt sign-in rather than calling the endpoint.

#### Scenario: Signed-in user votes from the job page

- **WHEN** a signed-in user taps thumbs up on a job detail page
- **THEN** the up counter increments, the up thumb is highlighted, and the vote
  endpoint is called

#### Scenario: Tapping the highlighted thumb clears it

- **WHEN** a user taps the thumb that is already highlighted as their vote
- **THEN** the vote is cleared and the counter decrements

#### Scenario: Anonymous visitor is prompted to sign in

- **WHEN** an anonymous visitor taps a thumb on a company page
- **THEN** a sign-in prompt is shown and no vote request is made

### Requirement: The vote control animates on activation

Activating a thumb SHALL play a brief, YouTube-style tactile animation on the
tapped thumb (a scale bounce / pop as it becomes the active vote), so the vote
feels responsive. The animation SHALL be purely presentational — it SHALL NOT
gate or delay the vote request or the counter update. The control SHALL respect
`prefers-reduced-motion`: when the user has requested reduced motion, the state
change SHALL apply instantly with no bounce.

#### Scenario: Thumb bounces when it becomes the active vote

- **WHEN** a signed-in user with default motion settings taps thumbs up
- **THEN** the up thumb plays a short scale-bounce animation as it becomes active

#### Scenario: Reduced motion suppresses the animation

- **WHEN** a user with `prefers-reduced-motion: reduce` taps a thumb
- **THEN** the thumb switches to its active state immediately with no bounce

#### Scenario: Animation does not block the vote

- **WHEN** a thumb is tapped
- **THEN** the counter and highlighted state update from the endpoint response
  independently of the animation's timing

