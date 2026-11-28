## ADDED Requirements

### Requirement: Tailoring places the vacancy on the Tracking Kanban

The system SHALL treat a successful CV-tailoring bootstrap for a vacancy as
placing that vacancy on the caller's Tracking Kanban: it bookmarks the vacancy
(`saved_at`) and, when no stage is set yet, sets stage to `applied` without
setting `applied_at`. The vacancy SHALL then appear in the Applied board column
(see `columnOf`: a stage is enough; silence remains unset without `applied_at`).

An existing non-empty stage SHALL be left unchanged. Clearing stage / save marks
afterwards SHALL NOT affect the tailored CV.

#### Scenario: A tailored vacancy appears in Applied

- **WHEN** a signed-in user successfully bootstraps tailoring for a vacancy with
  no prior stage
- **THEN** that vacancy is present in the caller's Tracking board listing with
  stage `applied` and no `applied_at`

#### Scenario: Progress is not overwritten

- **WHEN** a signed-in user who already staged the vacancy as `interview`
  bootstraps tailoring again
- **THEN** the stage remains `interview`
