## MODIFIED Requirements

### Requirement: Save a filter set
A signed-in user SHALL be able to save the current job-search filter state under a name. The saved set captures the canonical search query string (the same serialization the filter URL and `GET /api/v1/jobs/search` use), so re-applying it reproduces the exact filters, including an empty query string which represents "show all". A create request MAY mark the set as derived from the caller's candidate profile (`derived_from_profile`); at most one such set SHALL exist per user, and a create that would violate this SHALL be rejected rather than creating a second one.

#### Scenario: Create a saved search
- **WHEN** an authenticated user sends `POST /api/v1/me/searches` with a valid `name` and a `query` string
- **THEN** the system stores the set scoped to that user and responds `201` with `{"data": {id, name, query, updated_at}}`

#### Scenario: Empty query is valid
- **WHEN** an authenticated user creates a saved search with `query` equal to an empty string
- **THEN** the system stores it (it represents the unfiltered "show all" view) and responds `201`

#### Scenario: Unauthenticated request is rejected
- **WHEN** a request without a valid session cookie hits any `/api/v1/me/searches` endpoint
- **THEN** the system responds `401` and stores nothing

#### Scenario: Create a profile-derived saved search
- **WHEN** an authenticated user with no existing profile-derived saved search sends `POST /api/v1/me/searches` with `derived_from_profile: true`
- **THEN** the system stores the set with that flag set and responds `201`

#### Scenario: A second profile-derived saved search is rejected
- **WHEN** an authenticated user who already has a profile-derived saved search sends another `POST /api/v1/me/searches` with `derived_from_profile: true`
- **THEN** the system responds `409` and creates nothing
