## MODIFIED Requirements

### Requirement: The workspace is a three-column surface

The workspace SHALL lay out its ready state in three columns: a left panel tabbed between the CV
editor and the chat, a centre column showing the live CV preview, and a right panel tabbed
between templates, the job description, the job match, and the score. The left and right panels
SHALL be width-adjustable via draggable splitters clamped to a sensible range, with the centre
column taking the remaining width.

The right panel's tabs SHALL be divided by what each one measures, and no tab SHALL mix
measurements taken against different baselines:

- **Job Match** — the live job-anchored score of the tailored document against this vacancy, with
  the cached fit analysis beneath it kept current by the latest autopilot run, not a frozen
  snapshot;
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
