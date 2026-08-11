## ADDED Requirements

### Requirement: Project employments use name on the wire, company in storage

A `project`-kind employment SHALL serialize its place label as `name` (and MAY include `link`). It MUST omit `company` on the outbound JSON object. A `job`-kind employment SHALL continue to serialize the place label as `company` and MUST omit `name`.

Inbound JSON for a project SHALL accept `name`, and SHALL accept legacy `company` as a fallback into the same stored place-label field. Inbound JSON for a job SHALL take `company` only. Storage MAY keep a single place-label column for both kinds; clients MUST NOT assume projects expose `company`.

#### Scenario: A project reads back as name plus link

- **WHEN** the owner creates or lists a project-kind employment stored with place label `telagon.io` and a URL
- **THEN** the JSON object has `kind` `project`, `name` `telagon.io`, that `link`, and no `company` field

#### Scenario: A job still reads back as company

- **WHEN** the owner lists a job-kind employment
- **THEN** the JSON object has `company` and no `name` field

#### Scenario: Legacy project POST with company is accepted

- **WHEN** a caller posts a project-kind employment whose body sets `company` and omits `name`
- **THEN** the employment is stored and subsequent reads expose that label as `name`
