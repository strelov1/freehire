# tailor-workspace Specification

## Purpose
TBD - created by archiving change add-tailor-workspace. Update Purpose after archive.
## Requirements
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

The workspace SHALL offer the structured CV section form as one tab of the left panel, paired
with the chat tab, so the user switches between editing deterministic fields and talking to the
agent on the same side of the surface. Edits to a field MUST persist to the tailored CV (the same
CV the chat and preview show) AND reflect in the centre preview without a page reload.

#### Scenario: The editor tab edits the tailored CV

- **WHEN** the user opens the Editor tab and changes a field
- **THEN** the change persists to the tailored CV (the same CV the chat and centre preview show)

#### Scenario: Editing updates the centre preview live

- **WHEN** the user types into a field in the Editor tab
- **THEN** the centre CV preview re-renders to reflect the edit without a page reload or manual refresh

#### Scenario: Editor and chat are tabs of one panel

- **WHEN** the user is on the workspace
- **THEN** the left panel shows an Editor tab and a Chat tab, and selecting one shows that tab's content

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

### Requirement: The workspace is a three-column surface

The workspace SHALL lay out its ready state in three columns: a left panel tabbed between the CV
editor and the chat, a centre column showing the live CV preview, and a right panel tabbed
between templates, the job description, and the verdict. The left and right panels SHALL be
width-adjustable via draggable splitters clamped to a sensible range, with the centre column
taking the remaining width.

#### Scenario: The three columns render

- **WHEN** the workspace ready state renders on a wide viewport
- **THEN** the left tabbed panel (Editor/Chat), the centre CV preview, and the right tabbed panel (Templates/Job description/Verdict) are all visible side by side

#### Scenario: A side panel resizes and clamps

- **WHEN** the user drags a side-panel splitter beyond the allowed range
- **THEN** the panel width is clamped to the minimum/maximum rather than collapsing or overflowing, and the centre column absorbs the change

### Requirement: The centre column previews the CV as live HTML

The centre column SHALL render the tailored CV `Document` as HTML that updates as the CV changes
— instantly on a form edit (from the shared in-memory document) and after an agent turn (by
refetching the CV). The centre SHALL NOT embed a PDF; instead it SHALL offer a Download PDF action
that fetches the rendered PDF from the existing endpoint, and a zoom control that scales the
preview.

#### Scenario: The preview is HTML, not an embedded PDF

- **WHEN** the workspace renders the centre column
- **THEN** the CV is shown as HTML (no embedded PDF viewer), with a zoom control and a Download PDF button

#### Scenario: An agent turn refreshes the preview

- **WHEN** the agent completes a turn that edits the CV
- **THEN** the centre preview refetches and re-renders the updated CV

#### Scenario: Download PDF yields the rendered document

- **WHEN** the user activates Download PDF
- **THEN** the browser fetches the CV's rendered PDF from the existing per-CV PDF endpoint

### Requirement: The workspace offers a template picker

The workspace SHALL present a Templates tab in the right panel listing the registered CV
templates and letting the user select one; selecting a template SHALL set the tailored CV's
`template_id`, which the Download PDF output honours.

#### Scenario: Selecting a template sets the CV template

- **WHEN** the user picks a template in the Templates tab
- **THEN** the tailored CV's `template_id` is updated and the subsequent PDF download uses that template

#### Scenario: The registered templates are listed

- **WHEN** the user opens the Templates tab
- **THEN** it lists the registered templates (at minimum the default), with the CV's current template indicated

