## MODIFIED Requirements

### Requirement: Render a CV to an ATS-clean PDF on demand

The system SHALL render a CV to PDF on request and stream it as `application/pdf` without persisting the file. The rendered PDF SHALL contain a selectable text layer (not a rasterized image) with embedded fonts and without problematic ligatures, regardless of template. Templates marked ATS-safe SHALL additionally follow the ATS layout contract: a single column with standard section headings. Templates not marked ATS-safe MAY use richer layouts (such as a sidebar) as long as the text layer remains extractable. The render SHALL consult the CV's link-tracing flag and, when it is set, substitute the target of each eligible outbound link while leaving the text shown to a reader unchanged; the stored CV document SHALL NOT be modified by rendering.

#### Scenario: Download a CV PDF

- **WHEN** an authenticated user requests `GET /api/v1/me/cvs/:id/pdf` for a CV they own
- **THEN** the system renders the CV via the configured renderer and streams a `application/pdf` response

#### Scenario: Rendered text is extractable

- **WHEN** the CV PDF is rendered and its text layer is extracted
- **THEN** the candidate's name and skills appear as selectable text in the extracted output

#### Scenario: Non-ATS-safe template still yields extractable text

- **WHEN** a CV using a template not marked ATS-safe (e.g. `sidebar`) is rendered and its text layer is extracted
- **THEN** the candidate's name and skills still appear as selectable text

#### Scenario: A traced render leaves the extracted text alone

- **WHEN** a CV with link tracing enabled is rendered and its text layer is extracted
- **THEN** the extracted text carries the candidate's own link text, not the substituted target

#### Scenario: Rendering never mutates the stored document

- **WHEN** a CV with link tracing enabled is rendered
- **THEN** the CV document stored for that CV is byte-for-byte unchanged

## ADDED Requirements

### Requirement: Every template renders links as clickable links

Every template in the registry SHALL emit its links as clickable link annotations rather than
as plain text, so that a reader opening the PDF can follow them and so that a substituted
target is reachable. A template MUST NOT print a link as inert text.

#### Scenario: Each registered template emits clickable links

- **WHEN** a CV carrying header links and a project link is rendered with each template in the
  registry in turn
- **THEN** every rendered document exposes those links as clickable links

#### Scenario: A newly added template is held to the same rule

- **WHEN** a template is added to the registry
- **THEN** the registry-driven check covers it without being edited
