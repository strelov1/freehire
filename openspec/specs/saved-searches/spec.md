# saved-searches Specification

## Purpose
TBD - created by archiving change saved-searches. Update Purpose after archive.
## Requirements
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
A signed-in user SHALL be able to list their own saved searches, most recently updated first. Each listed set SHALL report its shared state: its public slug (empty when private) and author label.

#### Scenario: List own sets
- **WHEN** an authenticated user sends `GET /api/v1/me/searches`
- **THEN** the system responds `200` with `{"data": [...]}` containing only that user's saved searches ordered by `updated_at` descending, each including `public_slug` (empty when not shared) and `author_label`

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

The web filter modal SHALL present a "My filters" tab to signed-in users for
selecting, saving, updating, and deleting saved searches, and SHALL prompt anonymous
users to sign in instead of showing the list. Because the modal defers, the tab
SHALL operate on the staged filters: selecting a set seeds the staged state and
saving captures the staged filters; the changes reach the live filter state (and the
URL) only when the modal's **Show results** action is activated.

#### Scenario: Apply a saved search

- **WHEN** a signed-in user selects a saved set from the "My filters" tab and activates
  **Show results**
- **THEN** the modal parses the stored query string into the staged filters and, on
  **Show results**, commits it to the URL, and the results re-search accordingly

#### Scenario: Active set is marked

- **WHEN** the staged filter state's canonical query string equals a saved set's query
- **THEN** that set is marked active (checkmark) in the control

#### Scenario: Anonymous prompt

- **WHEN** an anonymous (signed-out) user opens the "My filters" tab
- **THEN** the control shows a "sign in to save" affordance that opens the auth dialog
  instead of a list of sets

### Requirement: Share a saved search as a public board
A signed-in user SHALL be able to make one of their own saved searches public by minting a stable public slug for it. Sharing MAY include an optional author label (free text, 1-60 characters after trim) stored on the board; an omitted or empty label means the board is anonymous. Sharing an already-shared set is idempotent for the slug (the existing slug is kept) but SHALL update the author label to the supplied value.

#### Scenario: Share a private saved search
- **WHEN** an authenticated user sends `POST /api/v1/me/searches/:id/share` for a set they own that has no public slug
- **THEN** the system generates a public slug from the set's name plus a short random suffix, stores it, and responds `200` with `{"data": {id, name, query, public_slug, author_label, ...}}`

#### Scenario: Share with an author label
- **WHEN** an authenticated user shares a set with a non-empty `author_label` in the body
- **THEN** the system stores the trimmed label and includes it in the response

#### Scenario: Re-share keeps the slug
- **WHEN** an authenticated user sends the share request for a set that already has a public slug
- **THEN** the system keeps the existing slug, applies any supplied author label, and responds `200`

#### Scenario: Over-long author label rejected
- **WHEN** the trimmed `author_label` exceeds 60 characters
- **THEN** the system responds `400` and does not change the set

#### Scenario: Cannot share another user's set
- **WHEN** an authenticated user sends the share request for a saved-search id owned by a different user
- **THEN** the system responds `404` and mints no slug

#### Scenario: Unauthenticated share rejected
- **WHEN** a request without a valid session cookie hits the share endpoint
- **THEN** the system responds `401` and changes nothing

### Requirement: Unshare a public board
A signed-in user SHALL be able to make one of their own shared saved searches private again, clearing its public slug. Unsharing invalidates the previously issued public link; a subsequent share mints a new slug rather than reviving the old one.

#### Scenario: Unshare a shared set
- **WHEN** an authenticated user sends `DELETE /api/v1/me/searches/:id/share` for a set they own that has a public slug
- **THEN** the system clears the public slug (and the board becomes unreachable by slug) and responds `204`

#### Scenario: Unshare a set that is not shared
- **WHEN** an authenticated user unshares a set they own that has no public slug
- **THEN** the system responds `204` (idempotent no-op)

#### Scenario: Cannot unshare another user's set
- **WHEN** an authenticated user unshares a saved-search id owned by a different user
- **THEN** the system responds `404` and the target set is unchanged

### Requirement: Public read of a shared board by slug
The system SHALL serve a shared board to anyone by its public slug without authentication, exposing only the board's name, canonical query, and author label. The owner's identity (user id, email) MUST NOT be exposed. A slug that does not exist or belongs to a set that is not currently shared SHALL be a 404.

#### Scenario: Read a shared board
- **WHEN** any client sends `GET /api/v1/boards/:slug` for a currently shared set
- **THEN** the system responds `200` with `{"data": {name, query, author_label}}` and no owner-identifying fields

#### Scenario: Unknown or unshared slug
- **WHEN** the slug does not exist, or names a set whose public slug has been cleared
- **THEN** the system responds `404`

#### Scenario: Anonymous board omits author
- **WHEN** a shared board has no author label
- **THEN** the response's `author_label` is empty (the board renders anonymously)

### Requirement: Public board page in the web app
The web app SHALL expose a public, unauthenticated route `/b/:slug` that loads a shared board and renders its jobs by applying the board's stored query to the jobs list, showing the board name and, when present, the author label. An unknown slug SHALL render a not-found state.

#### Scenario: Open a shared board link
- **WHEN** a visitor opens `/b/:slug` for a shared board
- **THEN** the page shows the board name (and author label if present) and lists the jobs matching the board's stored query

#### Scenario: Open an unknown board link
- **WHEN** a visitor opens `/b/:slug` for a slug that is not shared
- **THEN** the page renders a not-found state rather than an empty jobs list

### Requirement: Saved searches section in the account area
The web app SHALL expose a dedicated account section at `/my/searches`, reachable from the header account menu alongside the other `/my/*` sections, that lists the signed-in user's saved searches and lets them manage each one: share it as a public board (with an optional author label), unshare it, copy its public `/b/:slug` link when shared, rename it, and delete it. Creating a new saved search is out of scope for this section (it happens in the filters "My filters" control where the current filters exist). An anonymous visitor SHALL be prompted to sign in rather than shown a list.

#### Scenario: List and manage from the account section
- **WHEN** a signed-in user opens `/my/searches`
- **THEN** the page lists their saved searches, each showing whether it is shared, with actions to share, unshare, rename, delete, and (when shared) copy its public link

#### Scenario: Share from the account section
- **WHEN** a signed-in user shares a saved set from `/my/searches` (optionally supplying an author label) and confirms
- **THEN** the set is marked shared and its copyable public `/b/:slug` link is surfaced

#### Scenario: Unshare from the account section
- **WHEN** a signed-in user unshares a shared set from `/my/searches`
- **THEN** the set is marked private again and its public link is no longer offered

#### Scenario: Anonymous access to the section
- **WHEN** an anonymous (signed-out) visitor opens `/my/searches`
- **THEN** the page prompts sign-in instead of listing saved searches

