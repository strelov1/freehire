## MODIFIED Requirements

### Requirement: The workspace is a three-column surface

The workspace SHALL lay out its ready state in three columns: a left panel tabbed between the CV
editor and the chat, a centre column showing the live CV preview, and a right panel tabbed
between templates, the job description, the job match, the score, and the cover letter. The left
and right panels SHALL be width-adjustable via draggable splitters clamped to a sensible range,
with the centre column taking the remaining width.

The right panel's tabs SHALL be divided by what each one measures, and no tab SHALL mix
measurements taken against different baselines:

- **Job Match** — the live job-anchored score of the tailored document against this vacancy, with
  the cached fit analysis beneath it labelled as a snapshot of the base profile;
- **Score** — the ATS-readability delta of the tailored document against the base CV, and the log
  of the last autopilot run;
- **Cover letter** — the drafted letter for this vacancy. It is an artefact rather than a
  measurement, so it carries no baseline and SHALL NOT display a score of any kind;
- **Job description** and **Templates** — unchanged.

#### Scenario: The three columns render

- **WHEN** the workspace ready state renders on a wide viewport
- **THEN** the left tabbed panel (Editor/Chat), the centre CV preview, and the right tabbed panel (Templates/Job description/Job Match/Score/Cover letter) are all visible side by side

#### Scenario: A side panel resizes and clamps

- **WHEN** the user drags a side-panel splitter beyond the allowed range
- **THEN** the panel width is clamped to the minimum/maximum rather than collapsing or overflowing, and the centre column absorbs the change

#### Scenario: Each tab holds one baseline

- **WHEN** the user opens the Job Match tab
- **THEN** it shows the score measured against the vacancy, and the ATS-readability delta measured against the base CV is not shown there

#### Scenario: The cover letter tab shows no score

- **WHEN** the user opens the Cover letter tab
- **THEN** it shows the letter and its cited evidence, and no score or delta is rendered there

## ADDED Requirements

### Requirement: The cover letter tab reflects the draft's state

The Cover letter tab SHALL show, for the vacancy the workspace is tailoring for, either the stored
draft, an empty state offering to draft one, or the in-flight state of a draft being written. The
tab SHALL show which banked achievements the letter cites.

While a draft is in flight the tab SHALL NOT allow a second draft to be requested for the same
vacancy.

A stored draft reported stale SHALL be shown with its staleness, and SHALL remain readable and
copyable rather than being hidden or discarded.

#### Scenario: No draft exists yet

- **WHEN** the user opens the Cover letter tab for a vacancy with no stored draft
- **THEN** an empty state is shown with an action to draft one

#### Scenario: A draft is in flight

- **WHEN** a draft is being written
- **THEN** the tab shows the in-flight state and the draft action is unavailable until it settles

#### Scenario: A stale draft stays readable

- **WHEN** the stored draft's model stamp is stale
- **THEN** the letter is still shown and can be copied, with its staleness indicated

#### Scenario: The letter names its evidence

- **WHEN** a stored draft is shown
- **THEN** the banked achievements it cites are listed alongside it
