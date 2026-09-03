## MODIFIED Requirements

### Requirement: Retrieve the profile

A caller SHALL be able to fetch their single profile via `GET /api/v1/me/profile`,
authenticating with the session cookie or a full-scope API key. When
the user has saved a profile the system responds
`200` with `{"data": {specializations, skills, seniorities, excluded_skills, location_preferences, cv, created_at, updated_at}}`,
where `excluded_skills` is the saved set (an empty array when the user set none),
`seniorities` is the saved set (an empty array when the user set none), and
`location_preferences` is the saved block or `null` when the user set none; when
the user has no profile yet it responds `200` with `{"data": null}`.

The `cv` field carries the caller's structured résumé so a programmatic consumer can
read the user's professional history in the same call, and is `null` when the caller
has no current structured résumé (none stored, extraction unconfigured, not yet
extracted, or stale against the current CV). It SHALL be a whitelist projection that
omits the résumé's contact fields — `full_name`, `email`, `phone` and `links` — so a
field later added to the structured résumé is withheld until it is explicitly
projected. Contact details remain available only through `GET /api/v1/me/resume`.

#### Scenario: Fetch an existing profile
- **WHEN** an authenticated user who has a saved profile sends `GET /api/v1/me/profile`
- **THEN** the system responds `200` with `{"data": {...}}` containing that user's `specializations`, `skills`, `seniorities` (the saved set or an empty array), `excluded_skills` (the saved set or an empty array), `location_preferences` (the saved block or `null`), `cv`, and timestamps

#### Scenario: Fetch when no profile exists
- **WHEN** an authenticated user who has never saved a profile sends `GET /api/v1/me/profile`
- **THEN** the system responds `200` with `{"data": null}`

#### Scenario: The profile read accepts an API key
- **WHEN** a request carrying a valid API key as a bearer credential, and no session cookie, sends `GET /api/v1/me/profile`
- **THEN** the system responds `200` with the key owner's profile

#### Scenario: The cv block carries the structured résumé without contacts
- **WHEN** an authenticated user who has a saved profile and a current structured résumé sends `GET /api/v1/me/profile`
- **THEN** the response's `cv` carries the résumé's professional content — headline, location, summary, total years, experience, education, languages, skills, certifications and projects — and contains no `full_name`, `email`, `phone` or `links`

#### Scenario: The cv block is null without a structured résumé
- **WHEN** an authenticated user who has a saved profile but no current structured résumé sends `GET /api/v1/me/profile`
- **THEN** the response's `cv` is `null` and the rest of the profile is served as usual

### Requirement: Save the profile

A signed-in user SHALL be able to create-or-replace their single profile via
`PUT /api/v1/me/profile` with a non-empty set of `specializations` (job
categories), a non-empty set of `skills`, an optional set of `seniorities`
(experience levels), an optional set of `excluded_skills`, and an optional
`location_preferences` block. The write is an upsert keyed by the calling user: it
creates the profile if none exists and overwrites it otherwise. All skill sets are
stored trimmed and deduplicated as canonical lowercase tokens; `seniorities` MAY be
empty and defaults to empty when omitted; `excluded_skills` MAY be empty and
defaults to empty when omitted; the location block is validated and normalized per
the Location & work-mode preferences requirement, or stored as absent when
omitted. Any skill that appears in both `skills` and `excluded_skills` after
normalization SHALL be dropped from `excluded_skills` — a skill cannot be both
wanted and avoided, and the wanted set wins (no error is raised). The system does
NOT create an empty profile — a profile exists only once saved with valid content.

The response SHALL be the same representation the read serves, `cv` included, so a
client that saves and a client that fetches see one shape for one resource.

#### Scenario: Create the profile on first save
- **WHEN** an authenticated user with no profile sends `PUT /api/v1/me/profile` with a non-empty `specializations` array drawn from the category vocabulary and a non-empty `skills` array
- **THEN** the system stores the profile for that user and responds `200` with `{"data": {specializations, skills, seniorities, excluded_skills, location_preferences, cv, updated_at}}`

#### Scenario: Overwrite an existing profile
- **WHEN** an authenticated user who already has a profile sends `PUT /api/v1/me/profile` with new valid `specializations`, `skills`, `seniorities`, `excluded_skills`, and `location_preferences`
- **THEN** the system replaces the stored values (including the seniorities set, the excluded-skills set, and the location block), bumps `updated_at`, and responds `200`

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

#### Scenario: Seniorities may be empty
- **WHEN** an authenticated user saves a profile with valid `specializations` and `skills` and no `seniorities`
- **THEN** the system stores an empty `seniorities` set and the save succeeds

## ADDED Requirements

### Requirement: Seniorities validation

A profile's `seniorities` SHALL be a set of values drawn from the controlled seniority
vocabulary (`vocab.SeniorityValues`), each trimmed, with duplicates removed preserving
first-seen order. Unlike `specializations`, `seniorities` MAY be empty — a profile does
not require a stated level. There is no cap: the vocabulary itself is bounded (8
values), so no separate limit is needed.

#### Scenario: Unknown seniority rejected
- **WHEN** an authenticated user saves a profile whose `seniorities` contain a value that is not in the seniority vocabulary
- **THEN** the system responds `400` and stores nothing

#### Scenario: Seniorities are deduplicated
- **WHEN** an authenticated user saves a profile whose `seniorities` contain duplicate levels
- **THEN** the system stores each level once, preserving first-seen order

#### Scenario: A profile may span multiple seniority levels
- **WHEN** an authenticated user saves a profile with `seniorities` `[middle, senior]`
- **THEN** the system stores both levels and a subsequent fetch returns exactly those two
