## ADDED Requirements

### Requirement: Save a filter set
A signed-in user SHALL be able to save the current job-search filter state under a name. The saved set captures the canonical search query string (the same serialization the filter URL and `GET /api/v1/jobs/search` use), so re-applying it reproduces the exact filters, including an empty query string which represents "show all".

#### Scenario: Create a saved search
- **WHEN** an authenticated user sends `POST /api/v1/me/searches` with a valid `name` and a `query` string
- **THEN** the system stores the set scoped to that user and responds `201` with `{"data": {id, name, query, updated_at}}`

#### Scenario: Empty query is valid
- **WHEN** an authenticated user creates a saved search with `query` equal to an empty string
- **THEN** the system stores it (it represents the unfiltered "show all" view) and responds `201`

#### Scenario: Unauthenticated request is rejected
- **WHEN** a request without a valid session cookie hits any `/api/v1/me/searches` endpoint
- **THEN** the system responds `401` and stores nothing

### Requirement: Name validation
A saved search name SHALL be trimmed and contain between 1 and 100 characters, and SHALL be unique per user (case-sensitive after trim).

#### Scenario: Blank name rejected
- **WHEN** an authenticated user creates or renames a saved search with a name that is empty or only whitespace
- **THEN** the system responds `400` and stores nothing

#### Scenario: Over-long name rejected
- **WHEN** the trimmed name exceeds 100 characters
- **THEN** the system responds `400` and stores nothing

#### Scenario: Duplicate name rejected
- **WHEN** an authenticated user creates or renames a saved search to a name they already use
- **THEN** the system responds `409` and does not create or modify a row

### Requirement: Per-user cap
The system SHALL allow at most 50 saved searches per user.

#### Scenario: Cap exceeded on create
- **WHEN** an authenticated user who already has 50 saved searches sends a create request
- **THEN** the system responds `409` and stores nothing

### Requirement: List saved searches
A signed-in user SHALL be able to list their own saved searches, most recently updated first.

#### Scenario: List own sets
- **WHEN** an authenticated user sends `GET /api/v1/me/searches`
- **THEN** the system responds `200` with `{"data": [...]}` containing only that user's saved searches ordered by `updated_at` descending

### Requirement: Update a saved search
A signed-in user SHALL be able to overwrite a saved search's name and/or query. A field omitted from the request is left unchanged. The "Update" UI action overwrites the selected set's query with the current filters.

#### Scenario: Overwrite query
- **WHEN** an authenticated user sends `PATCH /api/v1/me/searches/:id` with a new `query`
- **THEN** the system replaces the stored query, bumps `updated_at`, and responds `200` with the updated row

#### Scenario: Rename
- **WHEN** an authenticated user sends `PATCH /api/v1/me/searches/:id` with a new `name`
- **THEN** the system replaces the stored name (subject to name validation) and responds `200`

### Requirement: Delete a saved search
A signed-in user SHALL be able to delete one of their own saved searches.

#### Scenario: Delete own set
- **WHEN** an authenticated user sends `DELETE /api/v1/me/searches/:id` for a set they own
- **THEN** the system removes the row and responds `204`

### Requirement: User-scoped access
Every saved-search operation SHALL be scoped to the calling user; one user MUST NOT be able to read, modify, or delete another user's saved search.

#### Scenario: Cannot touch another user's set
- **WHEN** an authenticated user sends `PATCH` or `DELETE` for a saved-search id owned by a different user
- **THEN** the system responds `404` and the target row is unchanged

### Requirement: Saved-search UI in the filters panel
The web filters panel SHALL present a "My filters" control to signed-in users for selecting, saving, updating, and deleting saved searches, and SHALL prompt anonymous users to sign in instead of showing the list.

#### Scenario: Apply a saved search
- **WHEN** a signed-in user selects a saved set from the "My filters" control
- **THEN** the panel applies its filters by parsing the stored query string into the filter state and committing it to the URL, and the results re-search accordingly

#### Scenario: Active set is marked
- **WHEN** the current filter state's canonical query string equals a saved set's query
- **THEN** that set is marked active (checkmark) in the control

#### Scenario: Anonymous prompt
- **WHEN** an anonymous (signed-out) user opens the filters panel
- **THEN** the "My filters" control shows a "sign in to save" affordance that opens the auth dialog instead of a list of sets
