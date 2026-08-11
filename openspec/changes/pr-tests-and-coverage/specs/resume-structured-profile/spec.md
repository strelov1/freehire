## ADDED Requirements

### Requirement: Profile résumé read composes bank jobs and projects

When the experience bank is configured, `GET /me/resume` SHALL place bank **job** employments into the composed structured `experience` list and bank **project** employments into `projects`, using the same SeedHistory split CV seed uses. Placeless publishable achievements SHALL appear as an experience entry that names no company or title and carries those claims as highlights. Automated tests MUST cover serving projects and placeless highlights through this surface.

#### Scenario: Bank projects appear on the résumé read

- **WHEN** the caller owns at least one `kind=project` employment with publishable atoms and requests `GET /me/resume`
- **THEN** the response structured payload includes those projects under `projects` (name/link/highlights), not only as flattened job rows

#### Scenario: Placeless claims trail experience

- **WHEN** the caller owns publishable atoms with no employment and requests `GET /me/resume`
- **THEN** structured `experience` includes an entry with empty place fields whose highlights are those claims

### Requirement: Retry parse refuses a missing stored object clearly

When the résumé pointer exists but the object store has no bytes for that key, retry parse SHALL NOT return a generic 500. It SHALL respond with a conflict (or equivalent client-correctable status) that tells the caller to upload the résumé again. Automated tests MUST cover mapping a missing-object error to that response.

#### Scenario: Missing blob on retry

- **WHEN** a signed-in caller retries parse and the store reports the object key is absent
- **THEN** the API responds with a conflict instructing them to upload again, and does not start extract
