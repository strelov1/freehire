## ADDED Requirements

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

### Requirement: Share affordance in the filters panel
The "My filters" control SHALL let a signed-in user share and unshare the selected saved search and obtain the public board link for a shared set, in addition to its existing create/apply/update/delete actions.

#### Scenario: Share from the control
- **WHEN** a signed-in user chooses to share a saved set and confirms
- **THEN** the control shows the set as shared and surfaces its copyable public `/b/:slug` link

#### Scenario: Unshare from the control
- **WHEN** a signed-in user unshares a shared set
- **THEN** the control shows the set as private again and no longer offers its public link

## MODIFIED Requirements

### Requirement: List saved searches
A signed-in user SHALL be able to list their own saved searches, most recently updated first. Each listed set SHALL report its shared state: its public slug (empty when private) and author label.

#### Scenario: List own sets
- **WHEN** an authenticated user sends `GET /api/v1/me/searches`
- **THEN** the system responds `200` with `{"data": [...]}` containing only that user's saved searches ordered by `updated_at` descending, each including `public_slug` (empty when not shared) and `author_label`
