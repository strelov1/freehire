## MODIFIED Requirements

### Requirement: Retrieve the profile

A caller SHALL be able to fetch their single profile via `GET /api/v1/me/profile`,
authenticating with the session cookie, an API key, or a bearer session token. When
the user has saved a profile the system responds `200` with
`{"data": {specializations, skills, excluded_skills, location_preferences, cv, created_at, updated_at}}`,
where `excluded_skills` is the saved set (an empty array when the user set none) and
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
- **THEN** the system responds `200` with `{"data": {...}}` containing that user's `specializations`, `skills`, `excluded_skills` (the saved set or an empty array), `location_preferences` (the saved block or `null`), `cv`, and timestamps

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

### Requirement: Session-scoped single profile

Every profile operation SHALL be scoped to the calling user, and each user SHALL
have at most one profile. There is no profile id in any path; the authenticated user
is the key. The read admits the session cookie, an API key, or a bearer session
token; the write and delete admit only the session cookie, so a leaked API key can
read a profile but never change or clear one.

#### Scenario: Unauthenticated request is rejected
- **WHEN** a request carrying no valid credential hits any `/api/v1/me/profile` endpoint
- **THEN** the system responds `401` and stores nothing

#### Scenario: An API key cannot write the profile
- **WHEN** a request carrying a valid API key and no session cookie sends `PUT /api/v1/me/profile` or `DELETE /api/v1/me/profile`
- **THEN** the system responds `401` and leaves any stored profile unchanged

#### Scenario: One profile per user
- **WHEN** an authenticated user who already has a profile saves again
- **THEN** the system still holds exactly one profile for that user (the saved values replace the previous ones)
