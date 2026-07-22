# search-profiles Specification (delta)

## MODIFIED Requirements

### Requirement: Retrieve the profile

A signed-in user SHALL be able to fetch their single profile via
`GET /api/v1/me/profile`. When the user has saved a profile the system responds
`200` with `{"data": {specializations, skills, excluded_skills, location_preferences, created_at, updated_at}}`,
where `excluded_skills` is the saved set (an empty array when the user set none) and
`location_preferences` is the saved block or `null` when the user set none; when
the user has no profile yet it responds `200` with `{"data": null}`.

#### Scenario: Fetch an existing profile
- **WHEN** an authenticated user who has a saved profile sends `GET /api/v1/me/profile`
- **THEN** the system responds `200` with `{"data": {...}}` containing that user's `specializations`, `skills`, `excluded_skills` (the saved set or an empty array), `location_preferences` (the saved block or `null`), and timestamps

#### Scenario: Fetch when no profile exists
- **WHEN** an authenticated user who has never saved a profile sends `GET /api/v1/me/profile`
- **THEN** the system responds `200` with `{"data": null}`

### Requirement: Save the profile

A signed-in user SHALL be able to create-or-replace their single profile via
`PUT /api/v1/me/profile` with a non-empty set of `specializations` (job
categories), a non-empty set of `skills`, an optional set of `excluded_skills`,
and an optional `location_preferences` block. The write is an upsert keyed by the
calling user: it creates the profile if none exists and overwrites it otherwise.
All skill sets are stored trimmed and deduplicated as canonical lowercase tokens;
`excluded_skills` MAY be empty and defaults to empty when omitted; the location
block is validated and normalized per the Location & work-mode preferences
requirement, or stored as absent when omitted. Any skill that appears in both
`skills` and `excluded_skills` after normalization SHALL be dropped from
`excluded_skills` — a skill cannot be both wanted and avoided, and the wanted set
wins (no error is raised). The system does NOT create an empty profile — a profile
exists only once saved with valid content.

#### Scenario: Create the profile on first save
- **WHEN** an authenticated user with no profile sends `PUT /api/v1/me/profile` with a non-empty `specializations` array drawn from the category vocabulary and a non-empty `skills` array
- **THEN** the system stores the profile for that user and responds `200` with `{"data": {specializations, skills, excluded_skills, location_preferences, updated_at}}`

#### Scenario: Overwrite an existing profile
- **WHEN** an authenticated user who already has a profile sends `PUT /api/v1/me/profile` with new valid `specializations`, `skills`, `excluded_skills`, and `location_preferences`
- **THEN** the system replaces the stored values (including the excluded-skills set and the location block), bumps `updated_at`, and responds `200`

#### Scenario: Specializations are deduplicated
- **WHEN** an authenticated user saves a profile whose `specializations` contain duplicate categories
- **THEN** the system stores each category once, preserving first-seen order

#### Scenario: Skills are normalized
- **WHEN** an authenticated user saves a profile with skills containing mixed case, surrounding whitespace, or duplicates
- **THEN** the system stores each skill lowercased, trimmed, and deduplicated

#### Scenario: Excluded skills are normalized
- **WHEN** an authenticated user saves a profile with `excluded_skills` containing mixed case, surrounding whitespace, or duplicates
- **THEN** the system stores each excluded skill lowercased, trimmed, and deduplicated

#### Scenario: A skill present in both sets is dropped from excluded skills
- **WHEN** an authenticated user saves a profile whose `skills` contain `go` and whose `excluded_skills` contain `go` and `php`
- **THEN** the system stores `excluded_skills` as `[php]` (the overlapping `go` is dropped) and the save succeeds

#### Scenario: Excluded skills may be empty
- **WHEN** an authenticated user saves a profile with valid `specializations` and `skills` and no `excluded_skills`
- **THEN** the system stores an empty `excluded_skills` set and the save succeeds

## ADDED Requirements

### Requirement: Edit excluded skills in the profile UI

The profile edit UI SHALL present a "skills to avoid" control, separate from the
wanted-skills control, that lets a signed-in user add and remove excluded skills.
The control SHALL be dictionary-constrained to canonical skill tokens (the same
skill vocabulary the wanted-skills control uses), so every excluded value matches
a real `skills` facet value. Excluded skills SHALL be optional — an empty set is
valid and does not affect the Save control's enabled state. The control SHALL be
pre-seeded with the user's currently saved excluded skills when editing.

#### Scenario: Add an excluded skill and save
- **WHEN** a signed-in user opens the profile editor, adds `php` to the "skills to avoid" control, and saves
- **THEN** the app calls `PUT /api/v1/me/profile` with `php` in `excluded_skills` and the saved profile reflects it

#### Scenario: Excluded skills are pre-seeded when editing
- **WHEN** a signed-in user who has saved excluded skills `[php]` reopens the profile editor
- **THEN** the "skills to avoid" control is pre-seeded with `php`

#### Scenario: Excluded skills do not gate saving
- **WHEN** a signed-in user has at least one specialization and one skill but no excluded skills
- **THEN** the Save control is enabled
