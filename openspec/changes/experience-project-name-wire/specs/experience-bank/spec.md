## ADDED Requirements

### Requirement: Project employments expose a name on the wire, not a company

On experience-bank HTTP surfaces that return or accept an employment, a row with `kind` equal to `project` SHALL carry its place label in the JSON field `name`. That response MUST NOT present the place label as `company`. A row with `kind` equal to `job` SHALL continue to use `company` (and optional `role`) as today.

On write (`POST` / `PUT` employments), a project MAY send `name`; a legacy `company` value for a project MUST still be accepted and stored as that same place label so existing clients keep working. Persistence MAY keep a single storage column for the label; the requirement binds the **wire** vocabulary to the kind, not the column name.

Assistant or tool projections that describe a banked place to the model or UI SHALL use the same kind-aware field names (`name` for projects, `company` for jobs).

#### Scenario: Listing a project returns name

- **WHEN** the owner lists experience and the bank holds a project whose place label is "telagon.io"
- **THEN** that employment's JSON includes `"kind":"project"` and `"name":"telagon.io"` and does not include a populated `company` for that label

#### Scenario: Listing a job still returns company

- **WHEN** the owner lists experience and the bank holds a job at RingCentral
- **THEN** that employment's JSON includes `"kind":"job"` and `"company":"RingCentral"`

#### Scenario: Creating a project with name works

- **WHEN** a caller posts an employment with `kind` project and `name` set
- **THEN** the project is persisted and the created entity is returned with `name`

#### Scenario: Creating a project with legacy company still works

- **WHEN** a caller posts an employment with `kind` project and `company` set (no `name`)
- **THEN** the place label is stored and returned as `name` on the response

#### Scenario: Experience UI labels a project by name

- **WHEN** the owner opens the experience view on a bank that includes a project
- **THEN** the project is shown under its name rather than under a “company” label
