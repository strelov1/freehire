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
between templates, the job description, the job match, and the score. The left and right panels
SHALL be width-adjustable via draggable splitters clamped to a sensible range, with the centre
column taking the remaining width.

The right panel's tabs SHALL be divided by what each one measures, and no tab SHALL mix
measurements taken against different baselines:

- **Job Match** — the live job-anchored score of the tailored document against this vacancy, with
  the cached fit analysis beneath it labelled as a snapshot of the base profile;
- **Score** — the ATS-readability delta of the tailored document against the base CV, and the log
  of the last autopilot run;
- **Job description** and **Templates** — unchanged.

#### Scenario: The three columns render

- **WHEN** the workspace ready state renders on a wide viewport
- **THEN** the left tabbed panel (Editor/Chat), the centre CV preview, and the right tabbed panel (Templates/Job description/Job Match/Score) are all visible side by side

#### Scenario: A side panel resizes and clamps

- **WHEN** the user drags a side-panel splitter beyond the allowed range
- **THEN** the panel width is clamped to the minimum/maximum rather than collapsing or overflowing, and the centre column absorbs the change

#### Scenario: Each tab holds one baseline

- **WHEN** the user opens the Job Match tab
- **THEN** it shows the score measured against the vacancy, and the ATS-readability delta measured against the base CV is not shown there

### Requirement: The centre column previews the CV as live HTML

The centre column SHALL render the tailored CV `Document` as HTML that updates as the CV changes
— instantly on a form edit (from the shared in-memory document) and after an agent turn (by
refetching the CV). The centre SHALL NOT embed a PDF; instead it SHALL offer a Download PDF action
that fetches the rendered PDF from the existing endpoint, and a zoom control that scales the
preview.

The preview SHALL render the CV as discrete A4 page sheets (page 1, page 2, …) rather than one
continuous column: it measures the rendered content and distributes top-level sections across
sheets at block boundaries, so a section is never split across the inter-page gap. Each sheet
applies the document's page margins as its padding, and the page body height used for pagination
is the A4 height minus the top and bottom margins. When the content exceeds one page, a second
(and further) sheet SHALL appear. For the two-column sidebar template, the main column paginates
across sheets while the narrow sidebar column renders on the first sheet.

#### Scenario: The preview is HTML, not an embedded PDF

- **WHEN** the workspace renders the centre column
- **THEN** the CV is shown as HTML (no embedded PDF viewer), with a zoom control and a Download PDF button

#### Scenario: Overflowing content paginates onto a second sheet

- **WHEN** the CV content is taller than one A4 page body
- **THEN** the preview shows a second A4 sheet and the section that would cross the page boundary starts at the top of the next sheet

#### Scenario: Margins drive the sheet layout

- **WHEN** the document's page margins change
- **THEN** each preview sheet's padding and the paginated page body height update to match

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

### Requirement: The workspace collapses to a single tabbed view on mobile

The workspace SHALL, below the `lg` breakpoint, collapse its three columns into
a single full-screen view selected by one flat, horizontally-scrollable tab bar
offering every view: Chat, Editor, Settings, Preview, Templates, Job, Job Match,
and Score. Selecting a tab SHALL show that view full-width and hide the others.
At `lg` and up the workspace SHALL render all three columns side by side as
before, and the flat mobile tab bar SHALL NOT be shown. The per-column tab bars
(Editor/Chat/Settings, Templates/Job/Job Match/Score) SHALL be desktop-only so
mobile navigation is not duplicated.

#### Scenario: The flat tab bar switches views on mobile

- **WHEN** the workspace renders on a narrow (below `lg`) viewport and the user taps a tab (e.g. Preview or Job Match)
- **THEN** that single view fills the screen and the other views are hidden, with the tab bar offering Chat, Editor, Settings, Preview, Templates, Job, Job Match, and Score

#### Scenario: Mobile selection stays consistent with the columns

- **WHEN** the user taps a mobile tab that corresponds to a column sub-tab (Editor, Chat, Settings, Templates, Job, Job Match, or Score)
- **THEN** the matching column's own tab is selected too, so switching to a wide viewport shows the same content selected

#### Scenario: The desktop layout is unchanged at lg

- **WHEN** the workspace renders at `lg` or wider
- **THEN** the three columns show side by side with their own tab bars and splitters, and the flat mobile tab bar is not shown

### Requirement: The account nav collapses to a drawer on mobile

The tailoring workspace SHALL, below the `lg` breakpoint, hide the fixed account
icon rail and instead expose the account sections through a burger button in the
mobile tab bar that opens a labelled slide-in drawer over a dimmed backdrop. The
drawer SHALL close on backdrop click, on `Escape`, on its close button, and after
a nav link is followed. At `lg` and up the account icon rail SHALL render as
before and no burger SHALL be shown.

#### Scenario: The burger opens the account drawer on mobile

- **WHEN** the workspace renders below `lg` and the user taps the burger in the mobile tab bar
- **THEN** a labelled drawer of account sections slides in over a dimmed backdrop, and the fixed icon rail is not shown

#### Scenario: The drawer dismisses

- **WHEN** the drawer is open and the user taps the backdrop, presses `Escape`, taps the close button, or follows a link
- **THEN** the drawer closes

#### Scenario: The rail is unchanged at lg

- **WHEN** the workspace renders at `lg` or wider
- **THEN** the account icon rail shows on the left edge as before and no burger button is present

### Requirement: The editor offers page-margin settings

The workspace editor (left panel) SHALL provide a Margins settings control with a stepper for each
side — top, right, bottom, left — labelled in inches, stepping by 0.05″ within the 0.25″–1.5″ range.
Adjusting a stepper SHALL update the shared in-memory document so the centre preview re-renders live
and the change is persisted by the existing autosave.

#### Scenario: Adjusting a margin updates the preview live

- **WHEN** the user increases the top margin with its stepper
- **THEN** the centre preview re-renders with the larger top margin without a page reload

#### Scenario: A margin change is autosaved

- **WHEN** the user changes any margin
- **THEN** the change is persisted through the workspace's existing autosave, and the Download PDF reflects it

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

### Requirement: The workspace links to the vacancy it is tailoring for

The workspace SHALL offer a link to the vacancy's own public page from its right-hand context
panel, so a candidate reading the job description can reach the posting, its company, its
application link and its fit analysis without navigating away by hand or editing the address bar.

The link SHALL open the vacancy's page rather than the outbound apply URL: the workspace's job is
to get the candidate back to what freehire knows about the role, not to send them to the employer
mid-tailoring. It SHALL be absent when the vacancy no longer exists, rather than rendering a link
to a dead page.

#### Scenario: The vacancy is one click away

- **WHEN** the workspace renders its context panel for a tailored CV bound to a live vacancy
- **THEN** a link to that vacancy's public page is shown, and following it lands on the job page

#### Scenario: A pruned vacancy shows no link

- **WHEN** the tailored CV's vacancy no longer exists
- **THEN** no vacancy link is rendered
