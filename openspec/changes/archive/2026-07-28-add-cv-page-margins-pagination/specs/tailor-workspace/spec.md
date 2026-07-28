## MODIFIED Requirements

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

## ADDED Requirements

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
