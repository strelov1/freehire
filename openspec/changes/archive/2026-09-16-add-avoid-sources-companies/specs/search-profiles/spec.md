## MODIFIED Requirements

### Requirement: Retrieve the profile

A caller SHALL be able to fetch their single profile via `GET /api/v1/me/profile`,
authenticating with the session cookie or a full-scope API key. When
the user has saved a profile the system responds
`200` with `{"data": {specializations, skills, excluded_skills, excluded_sources,
excluded_companies, location_preferences, cv, created_at, updated_at}}`,
where `excluded_skills`, `excluded_sources` and `excluded_companies` are each the
saved set (an empty array when the user set none) and `location_preferences` is the
saved block or `null` when the user set none; when the user has no profile yet it
responds `200` with `{"data": null}`.

The `cv` field carries the caller's structured résumé so a programmatic consumer can
read the user's professional history in the same call, and is `null` when the caller
has no current structured résumé (none stored, extraction unconfigured, not yet
extracted, or stale against the current CV). It SHALL be a whitelist projection that
omits the résumé's contact fields — `full_name`, `email`, `phone` and `links` — so a
field later added to the structured résumé is withheld until it is explicitly
projected. Contact details remain available only through `GET /api/v1/me/resume`.

#### Scenario: Fetch an existing profile

- **WHEN** an authenticated user who has a saved profile sends `GET /api/v1/me/profile`
- **THEN** the system responds `200` with `{"data": {...}}` containing that user's `specializations`, `skills`, `excluded_skills`, `excluded_sources`, `excluded_companies` (each the saved set or an empty array), `location_preferences` (the saved block or `null`), `cv`, and timestamps

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
categories), a non-empty set of `skills`, an optional set of `excluded_skills`, an
optional set of `excluded_sources`, an optional set of `excluded_companies`, and an
optional `location_preferences` block. The write is an upsert keyed by the
calling user: it creates the profile if none exists and overwrites it otherwise.
All skill sets are stored trimmed and deduplicated as canonical lowercase tokens;
`excluded_skills` MAY be empty and defaults to empty when omitted; `excluded_sources`
and `excluded_companies` are each stored trimmed, lowercased and deduplicated, and MAY
be empty and default to empty when omitted; the location block is validated and
normalized per the Location & work-mode preferences requirement, or stored as absent
when omitted. Any skill that appears in both `skills` and `excluded_skills` after
normalization SHALL be dropped from `excluded_skills` — a skill cannot be both wanted
and avoided, and the wanted set wins (no error is raised). `excluded_sources` and
`excluded_companies` have no corresponding "wanted" set, so no such overlap rule
applies to them. The system does NOT create an empty profile — a profile exists only
once saved with valid content.

The response SHALL be the same representation the read serves, `cv` included, so a
client that saves and a client that fetches see one shape for one resource.

#### Scenario: Create the profile on first save

- **WHEN** an authenticated user with no profile sends `PUT /api/v1/me/profile` with a non-empty `specializations` array drawn from the category vocabulary and a non-empty `skills` array
- **THEN** the system stores the profile for that user and responds `200` with `{"data": {specializations, skills, excluded_skills, excluded_sources, excluded_companies, location_preferences, cv, updated_at}}`

#### Scenario: Overwrite an existing profile

- **WHEN** an authenticated user who already has a profile sends `PUT /api/v1/me/profile` with new valid `specializations`, `skills`, `excluded_skills`, `excluded_sources`, `excluded_companies`, and `location_preferences`
- **THEN** the system replaces the stored values (including the excluded-skills, excluded-sources and excluded-companies sets and the location block), bumps `updated_at`, and responds `200`

#### Scenario: Specializations are deduplicated

- **WHEN** an authenticated user saves a profile whose `specializations` contain duplicate categories
- **THEN** the system stores each category once, preserving first-seen order

#### Scenario: Skills are normalized

- **WHEN** an authenticated user saves a profile with skills containing mixed case, surrounding whitespace, or duplicates
- **THEN** the system stores each skill lowercased, trimmed, and deduplicated

#### Scenario: Excluded skills are normalized

- **WHEN** an authenticated user saves a profile with `excluded_skills` containing mixed case, surrounding whitespace, or duplicates
- **THEN** the system stores each excluded skill lowercased, trimmed, and deduplicated

#### Scenario: Excluded sources are normalized

- **WHEN** an authenticated user saves a profile with `excluded_sources` containing mixed case, surrounding whitespace, or duplicates
- **THEN** the system stores each excluded source lowercased, trimmed, and deduplicated

#### Scenario: Excluded companies are normalized

- **WHEN** an authenticated user saves a profile with `excluded_companies` containing mixed case, surrounding whitespace, or duplicates
- **THEN** the system stores each excluded company slug lowercased, trimmed, and deduplicated

#### Scenario: A skill present in both sets is dropped from excluded skills

- **WHEN** an authenticated user saves a profile whose `skills` contain `go` and whose `excluded_skills` contain `go` and `php`
- **THEN** the system stores `excluded_skills` as `[php]` (the overlapping `go` is dropped) and the save succeeds

#### Scenario: Excluded skills may be empty

- **WHEN** an authenticated user saves a profile with valid `specializations` and `skills` and no `excluded_skills`
- **THEN** the system stores an empty `excluded_skills` set and the save succeeds

#### Scenario: Excluded sources may be empty

- **WHEN** an authenticated user saves a profile with valid `specializations` and `skills` and no `excluded_sources`
- **THEN** the system stores an empty `excluded_sources` set and the save succeeds

#### Scenario: Excluded companies may be empty

- **WHEN** an authenticated user saves a profile with valid `specializations` and `skills` and no `excluded_companies`
- **THEN** the system stores an empty `excluded_companies` set and the save succeeds

### Requirement: Skills validation

A profile's `skills` set SHALL be non-empty after normalization, and none of
`skills`, `excluded_skills`, `excluded_sources`, or `excluded_companies` SHALL exceed
200 entries after normalization.

The bound on `skills`/`excluded_skills` exists because that set is expanded per
element into the coverage verdict's search filter (one `skills != "<skill>"` AND
group each), so a stored list is a multiplier on every later read of the profile
against the index that also serves public search. `excluded_sources` and
`excluded_companies` carry the same 200-entry cap for consistency, though neither
feeds a per-element search filter today. A set past the bound SHALL be rejected with
`400` and nothing stored, exactly as a specialization set past its own cap is.

A single skill longer than 64 characters SHALL be dropped from the set rather than
rejecting the save, the same treatment blanks and duplicates receive: a per-value
problem does not fail an otherwise valid save, and no canonical skill the dictionary
emits approaches that length. A single excluded source or excluded company value
longer than 100 characters SHALL be dropped from its set the same way, rather than
rejecting the save.

#### Scenario: Empty skills rejected

- **WHEN** an authenticated user saves a profile whose `skills` are absent, empty, or reduce to empty after trimming
- **THEN** the system responds `400` and stores nothing

#### Scenario: Too many skills rejected

- **WHEN** an authenticated user saves a profile with more than 200 distinct skills, in either the wanted or the avoided set
- **THEN** the system responds `400` and stores nothing

#### Scenario: Too many excluded sources rejected

- **WHEN** an authenticated user saves a profile with more than 200 distinct `excluded_sources`
- **THEN** the system responds `400` and stores nothing

#### Scenario: Too many excluded companies rejected

- **WHEN** an authenticated user saves a profile with more than 200 distinct `excluded_companies`
- **THEN** the system responds `400` and stores nothing

#### Scenario: A skill set at the bound is stored whole

- **WHEN** an authenticated user saves a profile with exactly 200 distinct skills
- **THEN** the profile is stored with all 200

#### Scenario: An over-long value is dropped, not rejected

- **WHEN** an authenticated user saves a profile whose skills include a value longer than 64 characters alongside valid ones
- **THEN** the profile is stored with the over-long value omitted and the valid skills kept

#### Scenario: An over-long excluded source or company is dropped, not rejected

- **WHEN** an authenticated user saves a profile whose `excluded_sources` or `excluded_companies` include a value longer than 100 characters alongside valid ones
- **THEN** the profile is stored with the over-long value omitted and the valid values kept

### Requirement: Edit excluded skills, sources and companies in the profile UI

The profile UI SHALL present a dedicated "Avoid" tab (`/my/profile/avoid`), separate
from the "Skills" tab that holds the wanted-skills control, that lets a signed-in
user manage three independent exclusion lists: skills to avoid, sources to avoid,
and companies to avoid. Each control SHALL let the user add and remove entries and
SHALL be pre-seeded with the user's currently saved excluded values when the tab is
opened. None of the three controls gates the Save control's enabled state — all
three are optional, and an empty set is valid.

The "skills to avoid" control SHALL be dictionary-constrained to canonical skill
tokens (the same skill vocabulary the wanted-skills control on the Skills tab uses),
so every excluded value matches a real `skills` facet value. The "sources to avoid"
control SHALL be constrained to known `source` facet values (the same vocabulary the
general job-search source filter uses). The "companies to avoid" control SHALL be
constrained to known company identities, resolved the same way the general job-search
company filter resolves a company (by `company_slug`).

The "Skills" tab SHALL NOT present a "skills to avoid" control — that control lives
only on the Avoid tab.

#### Scenario: Add an excluded skill and save

- **WHEN** a signed-in user opens the Avoid tab, adds `php` to the "skills to avoid" control, and saves
- **THEN** the app calls `PUT /api/v1/me/profile` with `php` in `excluded_skills` and the saved profile reflects it

#### Scenario: Add an excluded source and save

- **WHEN** a signed-in user opens the Avoid tab, adds a source to the "sources to avoid" control, and saves
- **THEN** the app calls `PUT /api/v1/me/profile` with that source in `excluded_sources` and the saved profile reflects it

#### Scenario: Add an excluded company and save

- **WHEN** a signed-in user opens the Avoid tab, adds a company to the "companies to avoid" control, and saves
- **THEN** the app calls `PUT /api/v1/me/profile` with that company's slug in `excluded_companies` and the saved profile reflects it

#### Scenario: Excluded values are pre-seeded when reopening the tab

- **WHEN** a signed-in user who has saved `excluded_skills` `[php]`, `excluded_sources` `[greenhouse]`, and `excluded_companies` `[acme]` reopens the Avoid tab
- **THEN** the three controls are pre-seeded with `php`, `greenhouse`, and `acme` respectively

#### Scenario: Excluded values do not gate saving

- **WHEN** a signed-in user has at least one specialization and one skill but no excluded skills, sources, or companies
- **THEN** the Save control is enabled

#### Scenario: The Skills tab no longer shows an avoid control

- **WHEN** a signed-in user opens the "Skills" tab of their profile
- **THEN** it shows only the wanted-skills control, with no "skills to avoid" control on that tab
