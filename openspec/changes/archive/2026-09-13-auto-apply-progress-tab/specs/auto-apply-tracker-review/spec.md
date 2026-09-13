## ADDED Requirements

### Requirement: The tracker drawer shows a dedicated stage-progress view for an auto-apply attempt

The system SHALL present a candidate's live or already-submitted auto-apply attempt as a 3-stage
progress view — Tailoring, Review, Submitted — inside the tracker drawer, separate from the
drawer's general application view. This view SHALL be the one place the attempt's status text,
its approve/decline action, its answer-bank inputs, and its unmapped-questions list are shown;
the drawer's general application view SHALL NOT duplicate them.

#### Scenario: A tailoring attempt shows the first stage active

- **WHEN** the candidate opens the progress view for an attempt whose status is `tailoring`
- **THEN** the Tailoring stage is shown as in progress, and the Review and Submitted stages are
  shown as not yet reached

#### Scenario: A pending-review attempt shows the second stage active

- **WHEN** the candidate opens the progress view for an attempt whose status is `pending_review`
- **THEN** the Tailoring stage is shown as complete, the Review stage is shown as in progress
  (awaiting the candidate's decision), and the Submitted stage is shown as not yet reached

#### Scenario: An approved attempt shows the third stage as queued

- **WHEN** the candidate opens the progress view for an attempt whose status is `approved`
- **THEN** the Tailoring and Review stages are shown as complete, and the Submitted stage is
  shown as queued but not yet reached

#### Scenario: A stopped attempt marks the stage it stopped at

- **WHEN** the candidate opens the progress view for an attempt whose status is
  `tailor_failed` (stopped during Tailoring), `declined` (stopped during Review), `blocked`, or
  `failed` (stopped during Submitted)
- **THEN** the progress view marks the corresponding stage as stopped, and marks no later stage
  as reached

### Requirement: A successfully submitted auto-apply attempt is distinguishable from a manual application

Once an auto-apply attempt has been submitted, its live attempt record is retired and the job's
tracked application looks, by stage alone, like any other application. The system SHALL still let
the candidate tell, from the tracker drawer, that this particular application was submitted by
auto-apply rather than by hand.

#### Scenario: An application submitted by auto-apply shows the completed Submitted stage

- **WHEN** the candidate opens the progress view for a job whose auto-apply attempt has already
  submitted successfully (no live attempt remains)
- **THEN** the progress view shows all three stages — Tailoring, Review, Submitted — as complete

#### Scenario: A manually-applied job shows no progress view

- **WHEN** the candidate opens the tracker drawer for a job they applied to manually, which never
  had an auto-apply attempt
- **THEN** the drawer shows no progress view for that job

### Requirement: The candidate can mark a live attempt as paused for their own reference

The system SHALL let the candidate toggle a live, non-terminal auto-apply attempt (`tailoring`,
`pending_review`, `approved`, or `blocked`) between "paused" and "active" from the progress view,
as a personal reminder with no effect on the attempt's processing. This marker SHALL NOT change
`cmd/auto-apply`'s claim or processing behavior for the attempt, and SHALL NOT be offered once the
attempt has reached a terminal outcome (`declined`, `failed`, `tailor_failed`, or submitted).

#### Scenario: Pausing a live attempt does not affect its processing

- **WHEN** the candidate marks a `pending_review` or `approved` attempt as paused
- **THEN** the attempt's underlying status and eligibility for unattended submission are
  unchanged, and the progress view reflects the paused marker

#### Scenario: A terminal attempt offers no pause control

- **WHEN** the candidate opens the progress view for an attempt whose status is `declined`,
  `failed`, or `tailor_failed`, or for a job already submitted by auto-apply
- **THEN** no pause/continue control is offered
