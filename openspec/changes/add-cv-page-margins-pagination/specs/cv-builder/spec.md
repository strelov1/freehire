## ADDED Requirements

### Requirement: CV documents carry configurable page margins

A CV `Document` SHALL carry four page-margin values — top, right, bottom, and left — expressed
in inches. Margins are part of the document (not separate metadata), so they persist through the
existing CV storage, are copied when a CV is tailored, and are not clobbered by field-level
patches. Any margin the user has not set defaults to 0.5 inch.

#### Scenario: Unset margins default to half an inch

- **WHEN** a CV document is persisted with no margin values (or a zero value on a side)
- **THEN** that side's margin is treated as 0.5 inch

#### Scenario: Margins persist with the document

- **WHEN** a CV is saved with margins {top: 0.75, right: 0.5, bottom: 0.75, left: 0.5}
- **THEN** re-reading the CV returns those same margin values

### Requirement: Page margins are sanitized on persist

The CV sanitizer SHALL clamp each page margin to the range 0.25–1.5 inch, so a persisted CV
never carries an out-of-range or negative margin.

#### Scenario: An out-of-range margin is clamped

- **WHEN** a CV document is persisted with a left margin of 5.0 inch or -1.0 inch
- **THEN** the stored left margin is clamped to 1.5 inch (upper bound) or 0.25 inch (lower bound) respectively

### Requirement: Rendered PDF honours the CV's page margins

The Typst templates SHALL set the A4 page margins from the document's margin values (defaulting
to 0.5 inch per side), so the downloaded PDF reflects the user's chosen margins on every template.

#### Scenario: Custom margins reach the PDF

- **WHEN** a CV with margins {top: 1.0, right: 0.3, bottom: 1.0, left: 0.3} is rendered to PDF
- **THEN** the Typst compile applies those margins to the A4 page and produces a valid PDF
