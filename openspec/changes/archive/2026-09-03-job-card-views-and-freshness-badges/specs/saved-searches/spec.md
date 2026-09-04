## MODIFIED Requirements

### Requirement: Save a filter set
A signed-in user SHALL be able to save the current job-search filter state under a name. The saved set captures the canonical search query string (the same serialization the filter URL and `GET /api/v1/jobs/search` use), so re-applying it reproduces the exact filters, including an empty query string which represents "show all".

The canonical query SHALL NOT carry the feed's ordering. A saved search is about WHICH jobs are in the set, and the ordering does not change that: the digest matcher reads a stored query for its text and filters only and orders by its own clock, so two sets differing only by `sort` deliver identical mail. Including the ordering in the key therefore made a chosen ordering mark the saved search it came from as dirty, and saving again created a duplicate set that mailed the same jobs. Re-applying a saved search accordingly restores its filters and leaves the ordering at the contextual default.

#### Scenario: Create a saved search
- **WHEN** an authenticated user sends `POST /api/v1/me/searches` with a valid `name` and a `query` string
- **THEN** the system stores the set scoped to that user and responds `201` with `{"data": {id, name, query, updated_at}}`

#### Scenario: Empty query is valid
- **WHEN** an authenticated user creates a saved search with `query` equal to an empty string
- **THEN** the system stores it (it represents the unfiltered "show all" view) and responds `201`

#### Scenario: Unauthenticated request is rejected
- **WHEN** a request without a valid session cookie hits any `/api/v1/me/searches` endpoint
- **THEN** the system responds `401` and stores nothing

#### Scenario: Choosing an ordering does not fork the saved search
- **WHEN** a filter set saved while browsing is then reordered — for example to `Most viewed` — and compared against the stored set
- **THEN** the two canonical queries are equal, so the saved search still reads as active rather than dirty and saving again creates no duplicate

#### Scenario: The ordering is absent from the stored query
- **WHEN** a filter set carrying an explicit ordering is serialized as a saved search
- **THEN** the resulting query string carries no `sort` key, while every filter that decides which jobs are in the set is preserved
