## MODIFIED Requirements

### Requirement: Create a search profile
A signed-in user SHALL be able to create a named search profile capturing a non-empty set of `specializations` (one or more job categories) and a non-empty set of `skills`. Both sets are stored trimmed and deduplicated; skills are canonical lowercase tokens.

#### Scenario: Create a profile
- **WHEN** an authenticated user sends `POST /api/v1/me/profiles` with a valid `name`, a non-empty `specializations` array drawn from the category vocabulary, and a non-empty `skills` array
- **THEN** the system stores the profile scoped to that user and responds `201` with `{"data": {id, name, specializations, skills, updated_at}}`

#### Scenario: Specializations are deduplicated
- **WHEN** an authenticated user creates a profile whose `specializations` contain duplicate categories
- **THEN** the system stores each category once, preserving first-seen order

#### Scenario: Skills are normalized
- **WHEN** an authenticated user creates a profile with skills containing mixed case, surrounding whitespace, or duplicates
- **THEN** the system stores each skill lowercased, trimmed, and deduplicated

#### Scenario: Unauthenticated request is rejected
- **WHEN** a request without a valid session cookie hits any `/api/v1/me/profiles` endpoint
- **THEN** the system responds `401` and stores nothing

### Requirement: Update a search profile
A signed-in user SHALL be able to overwrite a profile's `name`, `specializations`, and/or `skills`. A field omitted from the request is left unchanged; a provided `specializations` or `skills` field MUST be non-empty after normalization.

#### Scenario: Overwrite skills
- **WHEN** an authenticated user sends `PATCH /api/v1/me/profiles/:id` with a new non-empty `skills` array
- **THEN** the system replaces the stored skills (normalized), bumps `updated_at`, and responds `200` with the updated row

#### Scenario: Change specializations
- **WHEN** an authenticated user sends `PATCH /api/v1/me/profiles/:id` with a new non-empty, valid `specializations` array
- **THEN** the system replaces the stored specializations and responds `200`

#### Scenario: Rename
- **WHEN** an authenticated user sends `PATCH /api/v1/me/profiles/:id` with a new `name`
- **THEN** the system replaces the stored name (subject to name validation) and responds `200`

### Requirement: Profile management UI
The web app SHALL present signed-in users a view to create, rename, edit, and delete their search profiles, and SHALL prompt anonymous users to sign in instead. The specialization input SHALL be a searchable multi-select over the category vocabulary; the skills input SHALL be a dictionary-backed typeahead that suggests matching canonical skills (with job counts) as the user types and adds each as a removable chip. The Create/Save control SHALL be enabled exactly when a name, at least one specialization, and at least one skill are present.

#### Scenario: Create a profile from the UI
- **WHEN** a signed-in user fills in a name, picks one or more specializations, adds one or more skills via the typeahead, and saves
- **THEN** the app calls `POST /api/v1/me/profiles` and shows the new profile in the list

#### Scenario: Skill typeahead suggests dictionary matches
- **WHEN** a signed-in user types into the skills field
- **THEN** the field lists matching canonical skills (with their job counts) and adds the chosen one as a removable chip; a non-matching query shows a "nothing found" hint rather than silently accepting an unknown skill

#### Scenario: Create control reflects completeness
- **WHEN** a signed-in user has entered a name, at least one specialization, and at least one skill
- **THEN** the Create/Save control is enabled; when any of the three is missing it is disabled

#### Scenario: Delete a profile from the UI
- **WHEN** a signed-in user deletes a profile from the list
- **THEN** the app calls `DELETE /api/v1/me/profiles/:id` and removes it from the list

#### Scenario: Anonymous prompt
- **WHEN** an anonymous (signed-out) user opens the profiles view
- **THEN** the view shows a "sign in" affordance instead of a profile list

## ADDED Requirements

### Requirement: Specializations validation
A profile's `specializations` SHALL be a non-empty set of values drawn from the controlled category vocabulary (`enrich.CategoryValues`), each trimmed, with duplicates removed, and the set capped at 5 entries.

#### Scenario: Unknown specialization rejected
- **WHEN** an authenticated user creates or updates a profile whose `specializations` contain a value that is not in the category vocabulary
- **THEN** the system responds `400` and stores nothing

#### Scenario: Empty specializations rejected
- **WHEN** an authenticated user creates a profile with no specializations, or updates one with a provided-but-empty `specializations` array
- **THEN** the system responds `400` and stores nothing

#### Scenario: Too many specializations rejected
- **WHEN** an authenticated user creates or updates a profile with more than 5 distinct specializations
- **THEN** the system responds `400` and stores nothing

## REMOVED Requirements

### Requirement: Specialization validation
**Reason**: The single-value `specialization` is replaced by the multi-valued `specializations` set; validation moves to the "Specializations validation" requirement.
**Migration**: Existing rows are migrated so each prior `specialization` becomes a single-element `specializations` set; clients send `specializations: [<category>]` instead of `specialization: <category>`.
