## MODIFIED Requirements

### Requirement: The tailoring workspace resumes an existing session

The system SHALL, when `/tailor/[slug]` is opened for an existing tailored CV (`?cv=<id>`),
re-attach to that CV's stored agent session WITHOUT bootstrapping a new CV or sending a kickoff
prompt. Opening `/tailor/[slug]` without a CV reference SHALL bootstrap a new tailored CV and
session and store the session id on it. A bootstrapped session MUST NOT start talking on its own:
the empty chat SHALL offer two actions — running the tailoring unattended, or walking the gaps in
conversation — and the turn begins when one is chosen.

#### Scenario: Re-opening a CV continues its conversation

- **WHEN** a user opens the workspace for an existing tailored CV
- **THEN** the existing agent session is attached (its prior messages replay) and no new session or kickoff is created

#### Scenario: Opening without a CV starts a fresh session

- **WHEN** a user opens the workspace from the match CTA (no CV reference)
- **THEN** a new tailored CV + seeded session are created, the session id is stored on the new CV, and the empty chat offers the two actions without sending anything

#### Scenario: Choosing an action starts the turn

- **WHEN** the user picks one of the two actions in the empty chat
- **THEN** the corresponding turn runs — the unattended run, or the conversational walkthrough

## ADDED Requirements

### Requirement: The editor is read-only while a run is in flight

The workspace SHALL prevent edits in the Editor tab for the duration of an autopilot run and say why,
because the tab holds its own copy of the document and saves it on a debounce: a run that edits the
same document server-side would race that save, and one side's work would be silently lost. Edits
MUST become possible again as soon as the run ends, on the document the run produced.

#### Scenario: The editor refuses edits mid-run

- **WHEN** an autopilot run is in flight and the user opens the Editor tab
- **THEN** the fields are not editable and the tab says the agent is editing the CV

#### Scenario: The editor reopens on the run's result

- **WHEN** the run ends
- **THEN** the Editor tab becomes editable again and shows the document the run produced
