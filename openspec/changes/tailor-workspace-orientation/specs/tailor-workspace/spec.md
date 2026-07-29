## MODIFIED Requirements

### Requirement: The tailoring workspace resumes an existing session

The system SHALL, when `/tailor/[slug]` is opened for an existing tailored CV (`?cv=<id>`),
re-attach to that CV's stored agent session WITHOUT bootstrapping a new CV or sending a kickoff
prompt. Opening `/tailor/[slug]` without a CV reference SHALL reach the tailored CV for that
vacancy and the conversation bound to it. A session MUST NOT start talking on its own: while its
conversation holds NO messages, the chat SHALL offer two actions — running the tailoring
unattended, or walking the gaps in conversation — and the turn begins when one is chosen.

The offer follows the conversation being empty, not the workspace being new. A CV re-opened by id
can carry a conversation nobody has spoken to, and leaving that case without actions makes it
indistinguishable from a conversation whose history was lost.

#### Scenario: Re-opening a CV continues its conversation

- **WHEN** a user opens the workspace for an existing tailored CV
- **THEN** the existing agent session is attached (its prior messages replay) and no new session or kickoff is created

#### Scenario: Opening without a CV reaches the vacancy's CV

- **WHEN** a user opens the workspace from the match CTA (no CV reference)
- **THEN** the tailored CV for that vacancy and its bound conversation are attached, and the empty chat offers the two actions without sending anything

#### Scenario: An empty resumed conversation still offers the way in

- **WHEN** a user opens a tailored CV by id whose conversation holds no messages
- **THEN** the chat offers the same two actions rather than an empty pane

#### Scenario: Choosing an action starts the turn

- **WHEN** the user picks one of the two actions in the empty chat
- **THEN** the corresponding turn runs — the unattended run, or the conversational walkthrough

## ADDED Requirements

### Requirement: The active pane is legible

The workspace SHALL mark the selected tab of each tabbed panel so it is unmistakable at a glance,
using the brand tint and a heavier weight rather than a fill that reads as background. A
three-column surface where the current pane is ambiguous costs the user the orientation the tabs
exist to give.

#### Scenario: The selected tab stands out

- **WHEN** a panel's tab is selected
- **THEN** it is rendered with the brand tint and heavier weight, and the unselected tabs are not
