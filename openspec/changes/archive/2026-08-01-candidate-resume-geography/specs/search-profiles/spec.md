## MODIFIED Requirements

### Requirement: Location & work-mode preferences

A profile SHALL be able to carry an optional `location_preferences` block recording both where the user **is** and where and how they **want** to work. The block is composed of the parts below — freely combinable and stored whole (as one value); every part and every field within it is optional, and a profile with no location preferences SHALL be represented by the block being absent (`null`). The block SHALL be:

- `work_modes`: a set of accepted work arrangements, each drawn from the controlled work-mode vocabulary (`enrich.WorkModeValues`: `remote`/`hybrid`/`onsite`), deduplicated.
- `remote`: the remote reach — `{ regions, countries }` — where `regions` is drawn from the controlled region vocabulary (`enrich.RegionValues`) and `countries` is a set of ISO 3166-1 alpha-2 codes; an empty reach means "worldwide".
- `base`: the user's current single location — `{ country, city }` — where `country` is an ISO 3166-1 alpha-2 code and `city` is free text. `base` is a **fact about the user**, not a preference: it states where they are, and is meaningful for every work arrangement. It MUST NOT be conditioned on which work modes the user accepts, and MUST NOT be dropped on save for a user who accepts only remote work — a remote worker's country still governs their right to work, their taxation, and their overlap with a team.
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

#### Scenario: A remote-only user keeps their base location
- **WHEN** an authenticated user whose `work_modes` are `[remote]` alone saves `location_preferences` with `base` `{country: co, city: "Manizales"}`
- **THEN** the system stores that base and a subsequent fetch returns it — it is not dropped for want of an on-site or hybrid work mode

## ADDED Requirements

### Requirement: The profile asks where the user is, independently of how they work

The profile edit UI SHALL present the "where you're based" control to every signed-in
user, unconditionally — it MUST NOT be revealed or hidden by which work modes the user
has accepted, and a base entered by a user who accepts only remote work MUST reach the
save request rather than being discarded with the hidden sub-form.

Where the system has derived a location from the user's CV and the user has not yet
stated a base, the control SHALL be pre-filled with the derived value so the user
confirms a fact already on their CV rather than retyping it. The pre-fill is a default
for an unstated field only: it MUST NOT overwrite a base the user has already saved, and
the saved value is whatever the user submits.

#### Scenario: A remote-only user is asked where they are based

- **WHEN** a signed-in user opens the profile editor and accepts only remote work
- **THEN** the "where you're based" control is visible and editable, and saving stores the
  country and city entered

#### Scenario: The base control is pre-filled from the derived geography

- **WHEN** a signed-in user with no saved base country, whose CV derives exactly one
  country, opens the profile editor
- **THEN** the base country control is pre-filled with the derived country, and saving
  without further edits stores it as their stated base

#### Scenario: A stated base is not overwritten by the derivation

- **WHEN** a signed-in user who has already saved a base country opens the profile editor
  and their CV derives a different country
- **THEN** the control shows their saved country, not the derived one

### Requirement: The profile read exposes the derived geography read-only

`GET /api/v1/me/profile` SHALL include a read-only block carrying the geography derived
from the caller's CV, alongside the stated `location_preferences`. The block MUST be
distinguishable from the stated preferences rather than merged into them — one is what
the candidate asserted and the other is what was derived for them, and a consumer needs
to know which it is holding. The block SHALL be null when nothing is derived, and MUST
never be writable through this or any other profile endpoint.

#### Scenario: A caller with derived geography receives it

- **WHEN** an authenticated user whose CV derives a country requests their profile
- **THEN** the response carries the derived countries, regions, and cities in their own
  block, separate from `location_preferences`

#### Scenario: A caller with no derived geography receives null

- **WHEN** an authenticated user with no current structured résumé requests their profile
- **THEN** the derived-geography block is `null`

#### Scenario: The derived block is not writable

- **WHEN** an authenticated user sends a profile save containing a derived-geography block
- **THEN** the stored derived geography is unchanged — it is produced only by the CV
  derivation, never by a profile write
