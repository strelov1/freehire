## ADDED Requirements

### Requirement: Tailoring places the vacancy on the Tracking Kanban

The system SHALL, on every successful tailor bootstrap for a vacancy
(`POST /api/v1/me/cvs/tailor`), ensure that vacancy appears on the caller's
Tracking Kanban board. Concretely it SHALL bookmark the vacancy (`saved_at`
set) and, when the vacancy has no application stage yet, set stage to
`applied`. It MUST NOT set `applied_at`: preparing a CV is not submitting an
application, and silence clocks MUST NOT start from the bootstrap alone.

The placement SHALL run for both a newly created tailored CV and an idempotent
resume of an existing one. An existing non-empty stage MUST be left unchanged.
Clearing board progress later MUST NOT delete or invalidate the tailored CV.

A failure to write tracking MUST NOT fail the bootstrap response once the
tailored CV and its session already exist.

#### Scenario: First tailor puts the vacancy in Applied

- **WHEN** a signed-in user successfully bootstraps tailoring for a vacancy they
  have never tailored
- **THEN** a tailored CV is created AND the vacancy is saved with stage
  `applied` and no `applied_at`

#### Scenario: Resume heals a missing board placement

- **WHEN** a signed-in user bootstraps tailoring for a vacancy they already have
  a tailored CV for, and that vacancy has no stage
- **THEN** the existing CV and conversation are returned AND the vacancy is
  staged as `applied` with `saved_at` set

#### Scenario: An advanced stage is preserved

- **WHEN** the vacancy already has stage `interview` and the user re-bootstraps
  tailoring
- **THEN** the stage remains `interview`

#### Scenario: Tailoring does not claim a submitted application

- **WHEN** a signed-in user successfully bootstraps tailoring for a vacancy
- **THEN** the vacancy's tracking interaction does not gain `applied_at` from
  the bootstrap alone

#### Scenario: A tracking write failure does not undo the tailored CV

- **WHEN** the tailored CV and session have been created and placing the vacancy
  on the board fails
- **THEN** the bootstrap still returns success with the tailored CV and session
  ids
