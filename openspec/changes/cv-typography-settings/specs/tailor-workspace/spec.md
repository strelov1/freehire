## ADDED Requirements

### Requirement: The editor offers typography settings

The workspace editor SHALL provide a Typography settings block with three controls: a font picker
listing the fonts the API reports, a stepper for the base font size in points, and a line-height
picker offering named density presets. Each control SHALL offer a "template default" choice that
clears the value so the active template decides it.

Adjusting any control SHALL update the shared in-memory document so the centre preview re-renders
live and the change is persisted by the existing autosave — the same path the margin steppers use.

The settings surface SHALL also offer a single action that clears every typography value at once,
returning the CV to the template's own presentation.

#### Scenario: Changing the font updates the preview live

- **WHEN** the user picks a different font
- **THEN** the centre preview re-renders in that face without a page reload, and the Download PDF
  reflects it once autosave completes

#### Scenario: Line height is chosen by name, not by number

- **WHEN** the user opens the line-height control
- **THEN** it offers named density presets rather than asking for a raw number

#### Scenario: Reset returns the CV to its template's presentation

- **WHEN** the user has changed font, size, and line height and then triggers the reset action
- **THEN** all three are cleared, and the preview and PDF render as the active template defines them

### Requirement: The live preview measures and draws with the same typography

The workspace's live HTML preview SHALL apply the document's typography to the hidden layer it
measures block heights in as well as to the sheets it draws, so pagination is computed under the same
type as it is rendered under. A change to font, size, or line height SHALL re-paginate the preview.

#### Scenario: Enlarging the type re-paginates the preview

- **WHEN** the user raises the base font size on a CV that fills one page
- **THEN** the preview re-measures and, if the content no longer fits, shows a second sheet

### Requirement: The settings surface holds at every panel width

Every control in the workspace's Settings tab SHALL lay out as a label-left, control-right row, so
that it holds at every width the left panel's splitter allows.

The Settings tab SHALL NOT size its controls off the viewport width. The panel is dragged
independently of the window, so a viewport breakpoint says nothing about the space a control actually
has — a wide window with a narrow panel is exactly the case that overflows.

#### Scenario: Settings hold at the narrowest panel width

- **WHEN** the user drags the left panel to its minimum width with the Settings tab open
- **THEN** every settings control stays inside the panel with no control clipped or overflowing

#### Scenario: Settings hold at the widest panel width

- **WHEN** the user drags the left panel to its maximum width with the Settings tab open
- **THEN** the controls keep the same label-left, control-right rhythm without stretching into
  unreadably wide rows

## MODIFIED Requirements

### Requirement: The editor offers page-margin settings

The workspace editor (left panel) SHALL provide a Margins settings control stepping by 0.05″ within
the 0.25″–1.5″ range and labelled in inches. Its default view SHALL be two linked steppers — the side
margins and the top-and-bottom margins — with the four independent per-side steppers available behind
a disclosure, because uniform margins are the common case and four steppers side by side do not fit
the panel at its narrow end.

Adjusting a stepper SHALL update the shared in-memory document so the centre preview re-renders live
and the change is persisted by the existing autosave.

#### Scenario: Adjusting a margin updates the preview live

- **WHEN** the user increases the top margin with its stepper
- **THEN** the centre preview re-renders with the larger top margin without a page reload

#### Scenario: A margin change is autosaved

- **WHEN** the user changes any margin
- **THEN** the change is persisted through the workspace's existing autosave, and the Download PDF reflects it

#### Scenario: A linked stepper moves both sides of its axis

- **WHEN** the user increases the side margins in the default view
- **THEN** both the left and the right margin change by the same step

#### Scenario: Per-side control is still reachable

- **WHEN** the user opens the per-side disclosure and changes only the top margin
- **THEN** only the top margin changes, and the linked view reflects that the axis is no longer uniform
