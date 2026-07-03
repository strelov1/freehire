## ADDED Requirements

### Requirement: Retrieve the profile

A signed-in user SHALL be able to fetch their single profile via
`GET /api/v1/me/profile`. When the user has saved a profile the system responds
`200` with `{"data": {specializations, skills, created_at, updated_at}}`; when
the user has no profile yet it responds `200` with `{"data": null}`.

#### Scenario: Fetch an existing profile
- **WHEN** an authenticated user who has a saved profile sends `GET /api/v1/me/profile`
- **THEN** the system responds `200` with `{"data": {...}}` containing that user's `specializations`, `skills`, and timestamps

#### Scenario: Fetch when no profile exists
- **WHEN** an authenticated user who has never saved a profile sends `GET /api/v1/me/profile`
- **THEN** the system responds `200` with `{"data": null}`

### Requirement: Save the profile

A signed-in user SHALL be able to create-or-replace their single profile via
`PUT /api/v1/me/profile` with a non-empty set of `specializations` (job
categories) and a non-empty set of `skills`. The write is an upsert keyed by the
calling user: it creates the profile if none exists and overwrites it otherwise.
Both sets are stored trimmed and deduplicated; skills are canonical lowercase
tokens. The system does NOT create an empty profile — a profile exists only once
saved with valid content.

#### Scenario: Create the profile on first save
- **WHEN** an authenticated user with no profile sends `PUT /api/v1/me/profile` with a non-empty `specializations` array drawn from the category vocabulary and a non-empty `skills` array
- **THEN** the system stores the profile for that user and responds `200` with `{"data": {specializations, skills, updated_at}}`

#### Scenario: Overwrite an existing profile
- **WHEN** an authenticated user who already has a profile sends `PUT /api/v1/me/profile` with new valid `specializations` and `skills`
- **THEN** the system replaces the stored values, bumps `updated_at`, and responds `200`

#### Scenario: Specializations are deduplicated
- **WHEN** an authenticated user saves a profile whose `specializations` contain duplicate categories
- **THEN** the system stores each category once, preserving first-seen order

#### Scenario: Skills are normalized
- **WHEN** an authenticated user saves a profile with skills containing mixed case, surrounding whitespace, or duplicates
- **THEN** the system stores each skill lowercased, trimmed, and deduplicated

### Requirement: Clear the profile

A signed-in user SHALL be able to delete their profile via
`DELETE /api/v1/me/profile`. The operation is idempotent.

#### Scenario: Delete an existing profile
- **WHEN** an authenticated user who has a profile sends `DELETE /api/v1/me/profile`
- **THEN** the system removes the row and responds `204`

#### Scenario: Delete when no profile exists
- **WHEN** an authenticated user with no profile sends `DELETE /api/v1/me/profile`
- **THEN** the system responds `204` and changes nothing

### Requirement: Session-scoped single profile

Every profile operation SHALL be scoped to the calling user via the session, and
each user SHALL have at most one profile. There is no profile id in any path;
the session user is the key.

#### Scenario: Unauthenticated request is rejected
- **WHEN** a request without a valid session cookie hits any `/api/v1/me/profile` endpoint
- **THEN** the system responds `401` and stores nothing

#### Scenario: One profile per user
- **WHEN** an authenticated user who already has a profile saves again
- **THEN** the system still holds exactly one profile for that user (the saved values replace the previous ones)

## MODIFIED Requirements

### Requirement: Specializations validation
A profile's `specializations` SHALL be a non-empty set of values drawn from the controlled category vocabulary (`enrich.CategoryValues`), each trimmed, with duplicates removed, and the set capped at 5 entries.

#### Scenario: Unknown specialization rejected
- **WHEN** an authenticated user saves a profile whose `specializations` contain a value that is not in the category vocabulary
- **THEN** the system responds `400` and stores nothing

#### Scenario: Empty specializations rejected
- **WHEN** an authenticated user saves a profile with no specializations
- **THEN** the system responds `400` and stores nothing

#### Scenario: Too many specializations rejected
- **WHEN** an authenticated user saves a profile with more than 5 distinct specializations
- **THEN** the system responds `400` and stores nothing

### Requirement: Skills validation
A profile's `skills` set SHALL be non-empty after normalization.

#### Scenario: Empty skills rejected
- **WHEN** an authenticated user saves a profile whose `skills` are absent, empty, or reduce to empty after trimming
- **THEN** the system responds `400` and stores nothing

### Requirement: Profile management UI
The web app SHALL present signed-in users a single view at `/my/profile` that shows their one profile (specialization and skill chips) and lets them edit or clear it, and SHALL prompt anonymous users to sign in instead. There is no profile name and no list of profiles. Editing SHALL happen in a modal that REUSES the job-search facet components (`FacetSection`) — the same specialization and skills controls the jobs filters use, sourcing the skills distribution from the live facet endpoint — rather than bespoke profile-only pickers. The profile facets SHALL disable the search-only exclude/match-any-or-all toggles (a profile value is neither excluded nor match-mode). The specialization selection SHALL be capped at 5. The Save control SHALL be enabled exactly when at least one specialization and at least one skill are present.

#### Scenario: Save the profile from the UI
- **WHEN** a signed-in user opens the edit modal, picks one or more specializations and skills via the shared facet controls, and saves
- **THEN** the app calls `PUT /api/v1/me/profile` and shows the saved profile

#### Scenario: Edit the existing profile
- **WHEN** a signed-in user who already has a profile opens the edit modal
- **THEN** the facet controls are pre-seeded with their current specializations and skills

#### Scenario: Skills use the shared job-search facet control
- **WHEN** a signed-in user edits skills in the profile modal
- **THEN** the control is the same `FacetSection` skills control as the jobs filters, listing canonical skills with their live job counts (not a separate profile-only typeahead)

#### Scenario: Save control reflects completeness
- **WHEN** a signed-in user has entered at least one specialization and at least one skill
- **THEN** the Save control is enabled; when either is missing it is disabled

#### Scenario: Anonymous prompt
- **WHEN** an anonymous (signed-out) user opens `/my/profile`
- **THEN** the view shows a "sign in" affordance instead of the profile

### Requirement: Populate profile skills from a resume

The profile form SHALL let a user upload a resume (PDF or pasted text) and
merge the extracted skills into the form's skills field. Extraction SHALL use the
`resume-skill-extraction` capability. Merging SHALL be a union with the skills already present
in the form (deduplicated); it SHALL NOT remove or overwrite skills the user already entered.
The user SHALL be able to edit or remove any skill chip before saving. Skills persist through
the profile save endpoint; no new persistence path is introduced.

#### Scenario: Merge extracted skills into an empty form

- **WHEN** a user with an empty skills field uploads a resume and extraction returns
  `[go, postgresql]`
- **THEN** the form's skills field contains exactly `[go, postgresql]`

#### Scenario: Merge extracted skills without wiping existing entries

- **WHEN** a user whose form already has `[docker]` uploads a resume returning `[go, docker, postgresql]`
- **THEN** the form's skills field contains the union `[docker, go, postgresql]` with no
  duplicates

#### Scenario: Extraction in progress shows a loading state

- **WHEN** the resume is being uploaded and analyzed
- **THEN** the upload control shows a loading/disabled state until extraction completes or fails

### Requirement: Display market skill-gap on a profile

The saved profile, when it has at least one specialization, SHALL display a skill-gap analysis
computed on the frontend against live market data. The frontend SHALL query
`GET /api/v1/jobs/facets` with the profile's specialization(s) as `category` filters (OR across
values), sort the returned `skills` facet by descending job count, and take the top N (N = 20)
as the expected skill set. Coverage SHALL be shown as `X/N`, where X is the count of expected
skills the profile already has. The skills in the expected set that the profile lacks SHALL be
shown as "missing" chips. The gap computation SHALL be a pure function
`computeGap(marketSkills, profileSkills, n)`.

#### Scenario: Coverage and missing skills for the profile

- **WHEN** the profile's specialization market top-20 skills are known and the profile has 13 of them
- **THEN** the view shows coverage `13/20` and lists the 7 missing skills as chips

#### Scenario: Multiple specializations combine markets

- **WHEN** the profile has specializations `[backend, devops]`
- **THEN** the expected skill set is derived from `/jobs/facets?category=backend&category=devops`
  (jobs in either category) taken as one combined top-20

#### Scenario: Profile without a specialization shows no gap block

- **WHEN** the profile has an empty specializations list
- **THEN** no skill-gap block is rendered

#### Scenario: Full coverage

- **WHEN** the profile already contains all top-20 expected skills
- **THEN** the view shows coverage `20/20` and lists no missing skills

## REMOVED Requirements

### Requirement: Create a search profile
**Reason**: Replaced by the singleton upsert "Save the profile" (`PUT /api/v1/me/profile`); creating named profiles by `POST` no longer exists.
**Migration**: Use `PUT /api/v1/me/profile` with `specializations` and `skills` (no `name`).

### Requirement: Name validation
**Reason**: A single per-user profile has no name.
**Migration**: The `name` field is dropped from the API and DB; clients stop sending it.

### Requirement: Per-user cap
**Reason**: Exactly one profile per user is enforced structurally (`user_profiles.user_id` is unique), so a 50-cap is meaningless.
**Migration**: None — the singleton is the cap.

### Requirement: List search profiles
**Reason**: There is nothing to list; a user has at most one profile fetched via `GET /api/v1/me/profile`.
**Migration**: Use `GET /api/v1/me/profile` (returns the profile or `null`).

### Requirement: Update a search profile
**Reason**: Replaced by the whole-profile upsert `PUT /api/v1/me/profile`; there is no id-addressed partial `PATCH`.
**Migration**: Send the full profile via `PUT /api/v1/me/profile`.

### Requirement: Delete a search profile
**Reason**: Replaced by the id-less `DELETE /api/v1/me/profile` (see "Clear the profile").
**Migration**: Use `DELETE /api/v1/me/profile`.

### Requirement: User-scoped access
**Reason**: Superseded by "Session-scoped single profile"; with no profile id there is no cross-user id to guard.
**Migration**: Ownership is implicit in the session; no id is accepted.
