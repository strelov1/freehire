## ADDED Requirements

### Requirement: Create a search profile

A signed-in user SHALL be able to create a named search profile capturing a single `specialization` (one job category) and a non-empty set of `skills`. Skills are stored as canonical lowercase tokens, trimmed and deduplicated.

#### Scenario: Create a profile

- **WHEN** an authenticated user sends `POST /api/v1/me/profiles` with a valid `name`, a `specialization` drawn from the category vocabulary, and a non-empty `skills` array
- **THEN** the system stores the profile scoped to that user and responds `201` with `{"data": {id, name, specialization, skills, updated_at}}`

#### Scenario: Skills are normalized

- **WHEN** an authenticated user creates a profile with skills containing mixed case, surrounding whitespace, or duplicates
- **THEN** the system stores each skill lowercased, trimmed, and deduplicated

#### Scenario: Unauthenticated request is rejected

- **WHEN** a request without a valid session cookie hits any `/api/v1/me/profiles` endpoint
- **THEN** the system responds `401` and stores nothing

### Requirement: Specialization validation

A profile's `specialization` SHALL be exactly one value from the controlled category vocabulary (`enrich.CategoryValues`).

#### Scenario: Unknown specialization rejected

- **WHEN** an authenticated user creates or updates a profile with a `specialization` that is not in the category vocabulary
- **THEN** the system responds `400` and stores nothing

#### Scenario: Missing specialization rejected

- **WHEN** an authenticated user creates a profile without a `specialization`
- **THEN** the system responds `400` and stores nothing

### Requirement: Skills validation

A profile's `skills` set SHALL be non-empty after normalization.

#### Scenario: Empty skills rejected

- **WHEN** an authenticated user creates or updates a profile whose `skills` are absent, empty, or reduce to empty after trimming
- **THEN** the system responds `400` and stores nothing

### Requirement: Name validation

A profile name SHALL be trimmed and contain between 1 and 100 characters, and SHALL be unique per user (case-sensitive after trim).

#### Scenario: Blank name rejected

- **WHEN** an authenticated user creates or renames a profile with a name that is empty or only whitespace
- **THEN** the system responds `400` and stores nothing

#### Scenario: Over-long name rejected

- **WHEN** the trimmed name exceeds 100 characters
- **THEN** the system responds `400` and stores nothing

#### Scenario: Duplicate name rejected

- **WHEN** an authenticated user creates or renames a profile to a name they already use
- **THEN** the system responds `409` and does not create or modify a row

### Requirement: Per-user cap

The system SHALL allow at most 50 search profiles per user.

#### Scenario: Cap exceeded on create

- **WHEN** an authenticated user who already has 50 profiles sends a create request
- **THEN** the system responds `409` and stores nothing

### Requirement: List search profiles

A signed-in user SHALL be able to list their own profiles, most recently updated first.

#### Scenario: List own profiles

- **WHEN** an authenticated user sends `GET /api/v1/me/profiles`
- **THEN** the system responds `200` with `{"data": [...]}` containing only that user's profiles ordered by `updated_at` descending

### Requirement: Update a search profile

A signed-in user SHALL be able to overwrite a profile's `name`, `specialization`, and/or `skills`. A field omitted from the request is left unchanged.

#### Scenario: Overwrite skills

- **WHEN** an authenticated user sends `PATCH /api/v1/me/profiles/:id` with a new non-empty `skills` array
- **THEN** the system replaces the stored skills (normalized), bumps `updated_at`, and responds `200` with the updated row

#### Scenario: Change specialization

- **WHEN** an authenticated user sends `PATCH /api/v1/me/profiles/:id` with a new valid `specialization`
- **THEN** the system replaces the stored specialization and responds `200`

#### Scenario: Rename

- **WHEN** an authenticated user sends `PATCH /api/v1/me/profiles/:id` with a new `name`
- **THEN** the system replaces the stored name (subject to name validation) and responds `200`

### Requirement: Delete a search profile

A signed-in user SHALL be able to delete one of their own profiles.

#### Scenario: Delete own profile

- **WHEN** an authenticated user sends `DELETE /api/v1/me/profiles/:id` for a profile they own
- **THEN** the system removes the row and responds `204`

### Requirement: User-scoped access

Every profile operation SHALL be scoped to the calling user; one user MUST NOT be able to read, modify, or delete another user's profile.

#### Scenario: Cannot touch another user's profile

- **WHEN** an authenticated user sends `PATCH` or `DELETE` for a profile id owned by a different user
- **THEN** the system responds `404` and the target row is unchanged

### Requirement: Profile management UI

The web app SHALL present signed-in users a view to create, rename, edit, and delete their search profiles, reusing the existing category and skill facet selectors for input, and SHALL prompt anonymous users to sign in instead.

#### Scenario: Create a profile from the UI

- **WHEN** a signed-in user fills in a name, picks a specialization, selects one or more skills, and saves
- **THEN** the app calls `POST /api/v1/me/profiles` and shows the new profile in the list

#### Scenario: Delete a profile from the UI

- **WHEN** a signed-in user deletes a profile from the list
- **THEN** the app calls `DELETE /api/v1/me/profiles/:id` and removes it from the list

#### Scenario: Anonymous prompt

- **WHEN** an anonymous (signed-out) user opens the profiles view
- **THEN** the view shows a "sign in" affordance instead of a profile list
