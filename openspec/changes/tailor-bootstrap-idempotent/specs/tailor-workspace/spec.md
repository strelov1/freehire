## ADDED Requirements

### Requirement: The workspace names its CV in the address

The workspace SHALL put the tailored CV's id into the address as soon as it has one, replacing the
current history entry rather than adding one. Until the address names the CV, a reload is a
bootstrap request — the page is addressed by vacancy — and the candidate is one refresh away from a
workspace that shows none of their conversation.

Replacing rather than pushing keeps Back leaving the workspace, which is where it went before.

#### Scenario: A reload resumes instead of bootstrapping

- **WHEN** the workspace has bootstrapped and the candidate reloads the page
- **THEN** the address carries the CV id, so the page re-attaches to that CV and its conversation

#### Scenario: Back still leaves the workspace

- **WHEN** the candidate presses Back after the address gained the CV id
- **THEN** they leave the workspace rather than stepping between two states of it
