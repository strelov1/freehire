## MODIFIED Requirements

### Requirement: The CV editor lives in the workspace

The workspace SHALL offer the structured CV section form as one tab of the left panel, alongside
the chat, an Experience tab, a Templates tab, and a Settings tab, so the user switches between
talking to the agent, editing deterministic fields, reviewing/confirming their banked experience,
picking a template, and tuning typography — all without leaving the left panel. Edits to a field
MUST persist to the tailored CV (the same CV the chat and preview show) AND reflect in the centre
preview without a page reload. The left panel's tabs SHALL appear in this order: Chat, Editor,
Experience, Templates, Settings.

#### Scenario: The editor tab edits the tailored CV

- **WHEN** the user opens the Editor tab and changes a field
- **THEN** the change persists to the tailored CV (the same CV the chat and centre preview show)

#### Scenario: Editing updates the centre preview live

- **WHEN** the user types into a field in the Editor tab
- **THEN** the centre CV preview re-renders to reflect the edit without a page reload or manual refresh

#### Scenario: The left panel's tabs appear in order

- **WHEN** the user is on the workspace
- **THEN** the left panel shows Chat, Editor, Experience, Templates, and Settings tabs in that
  order, and selecting one shows that tab's content

### Requirement: The workspace is a three-column surface

The workspace SHALL lay out its ready state in three columns: a left panel tabbed between Chat,
the CV editor, Experience, Templates, and Settings; a centre column showing the live CV preview;
and a right panel tabbed between templates, the job description, the job match, and the score.
The left and right panels SHALL be width-adjustable via draggable splitters clamped to a sensible
range, with the centre column taking the remaining width.

The right panel's tabs SHALL be divided by what each one measures, and no tab SHALL mix
measurements taken against different baselines:

- **Job Match** — the live job-anchored score of the tailored document against this vacancy, with
  the cached fit analysis beneath it labelled as a snapshot of the base profile;
- **Score** — the ATS-readability delta of the tailored document against the base CV, and the log
  of the last autopilot run;
- **Job description** and **Templates** — unchanged.

#### Scenario: The three columns render

- **WHEN** the workspace ready state renders on a wide viewport
- **THEN** the left tabbed panel (Chat/Editor/Experience/Templates/Settings), the centre CV
  preview, and the right tabbed panel (Templates/Job description/Job Match/Score) are all visible
  side by side

#### Scenario: A side panel resizes and clamps

- **WHEN** the user drags a side-panel splitter beyond the allowed range
- **THEN** the panel width is clamped to the minimum/maximum rather than collapsing or overflowing, and the centre column absorbs the change

#### Scenario: Each tab holds one baseline

- **WHEN** the user opens the Job Match tab
- **THEN** it shows the score measured against the vacancy, and the ATS-readability delta measured against the base CV is not shown there

### Requirement: The workspace collapses to a single tabbed view on mobile

The workspace SHALL, below the `lg` breakpoint, collapse its three columns into
a single full-screen view selected by one flat, horizontally-scrollable tab bar
offering every view: Chat, Editor, Experience, Settings, Preview, Templates, Job,
Job Match, and Score. Selecting a tab SHALL show that view full-width and hide the others.
At `lg` and up the workspace SHALL render all three columns side by side as
before, and the flat mobile tab bar SHALL NOT be shown. The per-column tab bars
(Chat/Editor/Experience/Settings, Templates/Job/Job Match/Score) SHALL be desktop-only so
mobile navigation is not duplicated.

#### Scenario: The flat tab bar switches views on mobile

- **WHEN** the workspace renders on a narrow (below `lg`) viewport and the user taps a tab (e.g. Preview or Job Match)
- **THEN** that single view fills the screen and the other views are hidden, with the tab bar offering Chat, Editor, Experience, Settings, Preview, Templates, Job, Job Match, and Score

#### Scenario: Mobile selection stays consistent with the columns

- **WHEN** the user taps a mobile tab that corresponds to a column sub-tab (Editor, Chat, Experience, Settings, Templates, Job, Job Match, or Score)
- **THEN** the matching column's own tab is selected too, so switching to a wide viewport shows the same content selected

#### Scenario: The desktop layout is unchanged at lg

- **WHEN** the workspace renders at `lg` or wider
- **THEN** the three columns show side by side with their own tab bars and splitters, and the flat mobile tab bar is not shown
