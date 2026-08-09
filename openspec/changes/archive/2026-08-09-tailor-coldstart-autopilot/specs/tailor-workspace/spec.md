## MODIFIED Requirements

### Requirement: The tailoring workspace resumes an existing session

The system SHALL, when `/tailor/[slug]` is opened for an existing tailored CV (`?cv=<id>`),
re-attach to that CV's stored agent session WITHOUT bootstrapping a new CV or sending a kickoff
prompt. Opening `/tailor/[slug]` without a CV reference SHALL bootstrap a new tailored CV and
session and store the session id on it. When the bootstrap response flags a first-time cold start
(per `cv-tailoring`'s "The bootstrap response flags a first-time cold start"), the workspace MUST
NOT offer a menu of actions before starting: it SHALL immediately start the autopilot run itself
(the same call the workspace's own "tailor it for me" action makes), and the empty chat instead
reflects the run in progress. The candidate MAY still send an ordinary message at any time,
including while the cold-start run is in flight.

#### Scenario: Re-opening a CV continues its conversation

- **WHEN** a user opens the workspace for an existing tailored CV
- **THEN** the existing agent session is attached (its prior messages replay) and no new session or kickoff is created

#### Scenario: Opening without a CV starts a fresh session and runs automatically

- **WHEN** a user opens the workspace for a vacancy with no tailored CV yet (no CV reference)
- **THEN** a new tailored CV + seeded session are created, the session id is stored on the new CV,
  and the cold-start autopilot run begins immediately without the candidate choosing an action first

## ADDED Requirements

### Requirement: The CV preview updates live during a cold-start run

The system SHALL refresh the displayed CV document after each `cv_edit` tool call an autopilot run
makes, not only once at the end of the turn, so the candidate sees bullets and sections fill in as
the run progresses. This applies to every autopilot run (cold-start or manually started), reusing
the same document refresh already performed at turn end.

#### Scenario: A bullet appears as the run writes it

- **WHEN** an autopilot run's `cv_edit` tool call successfully rewrites a bullet
- **THEN** the CV preview reflects that change before the run's turn has finished

#### Scenario: The end-of-turn refresh still runs

- **WHEN** an autopilot run's turn completes
- **THEN** the CV preview is refreshed once more, covering any change whose per-call refresh was
  missed
