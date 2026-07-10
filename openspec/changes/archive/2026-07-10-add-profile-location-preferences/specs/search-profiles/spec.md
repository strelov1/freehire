# search-profiles Specification (delta)

## ADDED Requirements

### Requirement: Location & work-mode preferences

A profile MAY carry an optional `location_preferences` block describing *where* and *how* the user wants to work. The block is composed of three independent, freely combinable parts and is stored whole (as one value); every part and every field within it is optional, and a profile with no location preferences SHALL be represented by the block being absent (`null`). The block SHALL be:

- `work_modes`: a set of accepted work arrangements, each drawn from the controlled work-mode vocabulary (`enrich.WorkModeValues`: `remote`/`hybrid`/`onsite`), deduplicated.
- `remote`: the remote reach — `{ regions, countries }` — where `regions` is drawn from the controlled region vocabulary (`enrich.RegionValues`) and `countries` is a set of ISO 3166-1 alpha-2 codes; an empty reach means "worldwide".
- `base`: the user's current single location — `{ country, city }` — where `country` is an ISO 3166-1 alpha-2 code and `city` is free text.
- `relocation`: willingness to move — `{ open, regions, countries, cities }` — where `open` is a boolean and the target `regions`/`countries`/`cities` are the acceptable destinations; `open` with empty targets means "anywhere".

On save the system SHALL validate and normalize the block: work modes and regions matched case-insensitively and rejected if outside their controlled vocabularies; countries lowercased and rejected if not a well-formed ISO 3166-1 alpha-2 shape (exactly two ASCII letters — shape, not assignment, so the full range of codes a user may pick is accepted); every code/token trimmed and deduplicated; cities trimmed, empty entries dropped, deduplicated, and capped. An invalid value SHALL reject the whole save (the profile is unchanged); nothing out-of-vocabulary is ever stored.

#### Scenario: Save a profile with combined location preferences
- **WHEN** an authenticated user saves a profile with `location_preferences` of `work_modes` `[remote, onsite]`, `remote.regions` `[latam]`, `base` `{country: br, city: "Florianópolis"}`, and `relocation` `{open: true, cities: ["Berlin"]}`
- **THEN** the system stores the block whole and a subsequent fetch returns exactly those preferences

#### Scenario: Location preferences are optional
- **WHEN** an authenticated user saves a profile with valid `specializations` and `skills` and no `location_preferences`
- **THEN** the system stores the profile with an absent (`null`) location block and the save succeeds

#### Scenario: Out-of-vocabulary work mode or region is rejected
- **WHEN** an authenticated user saves `location_preferences` whose `work_modes` or any `regions` entry is not in the controlled vocabulary
- **THEN** the system responds `400`, stores nothing, and leaves any existing profile unchanged

#### Scenario: Malformed country code is rejected
- **WHEN** an authenticated user saves `location_preferences` with a `countries` or `base.country` value that is not a two-letter code (e.g. an alpha-3 code like `usa`)
- **THEN** the system responds `400` and stores nothing

#### Scenario: Countries and cities are normalized
- **WHEN** an authenticated user saves `location_preferences` with country codes in mixed case and cities with surrounding whitespace or duplicates
- **THEN** the system stores country codes lowercased and deduplicated, and cities trimmed, non-empty, and deduplicated

## MODIFIED Requirements

### Requirement: Retrieve the profile

A signed-in user SHALL be able to fetch their single profile via
`GET /api/v1/me/profile`. When the user has saved a profile the system responds
`200` with `{"data": {specializations, skills, location_preferences, created_at, updated_at}}`,
where `location_preferences` is the saved block or `null` when the user set none; when
the user has no profile yet it responds `200` with `{"data": null}`.

#### Scenario: Fetch an existing profile
- **WHEN** an authenticated user who has a saved profile sends `GET /api/v1/me/profile`
- **THEN** the system responds `200` with `{"data": {...}}` containing that user's `specializations`, `skills`, `location_preferences` (the saved block or `null`), and timestamps

#### Scenario: Fetch when no profile exists
- **WHEN** an authenticated user who has never saved a profile sends `GET /api/v1/me/profile`
- **THEN** the system responds `200` with `{"data": null}`

### Requirement: Save the profile

A signed-in user SHALL be able to create-or-replace their single profile via
`PUT /api/v1/me/profile` with a non-empty set of `specializations` (job
categories), a non-empty set of `skills`, and an optional `location_preferences`
block. The write is an upsert keyed by the calling user: it creates the profile
if none exists and overwrites it otherwise. Both sets are stored trimmed and
deduplicated; skills are canonical lowercase tokens; the location block is
validated and normalized per the Location & work-mode preferences requirement, or
stored as absent when omitted. The system does NOT create an empty profile — a
profile exists only once saved with valid content.

#### Scenario: Create the profile on first save
- **WHEN** an authenticated user with no profile sends `PUT /api/v1/me/profile` with a non-empty `specializations` array drawn from the category vocabulary and a non-empty `skills` array
- **THEN** the system stores the profile for that user and responds `200` with `{"data": {specializations, skills, location_preferences, updated_at}}`

#### Scenario: Overwrite an existing profile
- **WHEN** an authenticated user who already has a profile sends `PUT /api/v1/me/profile` with new valid `specializations`, `skills`, and `location_preferences`
- **THEN** the system replaces the stored values (including the location block), bumps `updated_at`, and responds `200`

#### Scenario: Specializations are deduplicated
- **WHEN** an authenticated user saves a profile whose `specializations` contain duplicate categories
- **THEN** the system stores each category once, preserving first-seen order

#### Scenario: Skills are normalized
- **WHEN** an authenticated user saves a profile with skills containing mixed case, surrounding whitespace, or duplicates
- **THEN** the system stores each skill lowercased, trimmed, and deduplicated
