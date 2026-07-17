## ADDED Requirements

### Requirement: A tailored CV remembers its agent session

The system SHALL persist the agent session id bound to a tailored CV and return it on the CV
reads, so the CV can re-open its exact session later. Writing the session id MUST be owner-scoped
(a caller can only set it on their own CV).

#### Scenario: The session id round-trips on a tailored CV

- **WHEN** the owner sets the agent session id on their tailored CV and then reads the CV
- **THEN** the read returns that session id

#### Scenario: A foreign caller cannot set the session

- **WHEN** a caller sets the session id on a CV they do not own
- **THEN** the write is rejected (not found) and the CV is unchanged

### Requirement: The tailoring workspace resumes an existing session

The system SHALL, when `/tailor/[slug]` is opened for an existing tailored CV (`?cv=<id>`),
re-attach to that CV's stored agent session WITHOUT bootstrapping a new CV or sending a kickoff
prompt. Opening `/tailor/[slug]` without a CV reference SHALL bootstrap a new tailored CV and
session and store the session id on it.

#### Scenario: Re-opening a CV continues its conversation

- **WHEN** a user opens the workspace for an existing tailored CV
- **THEN** the existing agent session is attached (its prior messages replay) and no new session or kickoff is created

#### Scenario: Opening without a CV starts a fresh session

- **WHEN** a user opens the workspace from the match CTA (no CV reference)
- **THEN** a new tailored CV + seeded session are created, the agent auto-starts, and the session id is stored on the new CV

### Requirement: The CV editor lives in the workspace

The workspace SHALL offer the structured CV editor as a tab alongside the CV preview, job
description, and verdict, so the user edits the same tailored CV beside the chat.

#### Scenario: The editor tab edits the tailored CV

- **WHEN** the user opens the Edit tab and changes a field
- **THEN** the change persists to the tailored CV (the same CV the chat and preview show)

### Requirement: The CV list re-opens sessions and has no create action

The CV list SHALL show the user's tailored CVs, each linking to its tailoring workspace
(`/tailor/[slug]?cv=<id>`, resume), and SHALL NOT offer a create action — a tailored CV is
created only from the match page. The list MUST carry the job slug and the session id needed to
build each re-open link.

#### Scenario: A list item re-opens its workspace

- **WHEN** the user clicks a tailored CV in the list
- **THEN** they land on that CV's tailoring workspace with its existing session

#### Scenario: There is no create button

- **WHEN** the user views the CV list
- **THEN** no "create CV" action is shown
