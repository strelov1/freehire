## MODIFIED Requirements

### Requirement: The workspace is a three-column surface

The workspace SHALL lay out its ready state in three columns: a left panel tabbed between the CV
editor, the templates, the appearance settings and the chat, a centre column showing the live CV
preview, and a right panel tabbed between the job description, the job match, and the score. The
left and right panels SHALL be width-adjustable via draggable splitters clamped to a sensible
range, with the centre column taking the remaining width.

The left panel SHALL hold what CHANGES the document — its text, its template, its typography —
and the right panel SHALL hold what MEASURES it. The template gallery belongs to the first: it
decides how the CV looks, exactly as the font and the margins do, and nothing about it compares
the document to anything.

The right panel's tabs SHALL be divided by what each one measures, and no tab SHALL mix
measurements taken against different baselines:

- **Job Match** — the live job-anchored score of the tailored document against this vacancy, with
  the cached fit analysis beneath it labelled as a snapshot of the base profile;
- **Score** — the ATS-readability delta of the tailored document against the base CV, and the log
  of the last autopilot run;
- **Job description** — unchanged.

#### Scenario: The three columns render

- **WHEN** the workspace ready state renders on a wide viewport
- **THEN** the left tabbed panel (Editor/Templates/Settings/Chat), the centre CV preview, and the right tabbed panel (Job description/Job Match/Score) are all visible side by side

#### Scenario: A side panel resizes and clamps

- **WHEN** the user drags a side-panel splitter beyond the allowed range
- **THEN** the panel width is clamped to the minimum/maximum rather than collapsing or overflowing, and the centre column absorbs the change

#### Scenario: Choosing a template from the left panel

- **WHEN** the user opens the Templates tab and picks a template
- **THEN** the centre preview re-renders in that template and the choice is stored against the CV
