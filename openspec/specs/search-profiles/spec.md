# search-profiles Specification

## Purpose

A user profile is the user's single professional self — a non-empty set of specializations (job categories) and a non-empty set of skills. One per user (keyed by the session, no id, no name); the foundation for finding relevant work. This capability covers only the profile entity and its management (fetch/save/clear); how a profile is consumed (match scoring, ranked feeds, notifications) is out of scope here.
## Requirements
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

