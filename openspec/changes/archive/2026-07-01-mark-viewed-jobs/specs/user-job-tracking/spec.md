## ADDED Requirements

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
