## MODIFIED Requirements

### Requirement: Render a CV to an ATS-clean PDF on demand

The system SHALL render a CV to PDF on request and stream it as `application/pdf` without persisting the file. The rendered PDF SHALL contain a selectable text layer (not a rasterized image) with embedded fonts and without problematic ligatures, regardless of template. Templates marked ATS-safe SHALL additionally follow the ATS layout contract: a single column with standard section headings. Templates not marked ATS-safe MAY use richer layouts (such as a sidebar) as long as the text layer remains extractable.

#### Scenario: Download a CV PDF

- **WHEN** an authenticated user requests `GET /api/v1/me/cvs/:id/pdf` for a CV they own
- **THEN** the system renders the CV via the configured renderer and streams a `application/pdf` response

#### Scenario: Rendered text is extractable

- **WHEN** the CV PDF is rendered and its text layer is extracted
- **THEN** the candidate's name and skills appear as selectable text in the extracted output

#### Scenario: Non-ATS-safe template still yields extractable text

- **WHEN** a CV using a template not marked ATS-safe (e.g. `sidebar`) is rendered and its text layer is extracted
- **THEN** the candidate's name and skills still appear as selectable text

### Requirement: Template selection with an extensible registry

Each CV SHALL reference a template by `template_id`, defaulting to the ATS template `classic-ats`. The system SHALL resolve `template_id` through a template registry that carries multiple templates so additional templates can be added without schema changes. Each registered template SHALL have a stable id and a matching Typst template file. An unknown `template_id` SHALL be rejected rather than rendered.

#### Scenario: Default template applied

- **WHEN** a CV is created without an explicit template
- **THEN** its `template_id` defaults to `classic-ats` and renders with that template

#### Scenario: A non-default registered template renders

- **WHEN** a user sets a CV's `template_id` to another registered template (e.g. `centered`, `modern-sans`, or `sidebar`)
- **THEN** the CV is accepted and rendered with that template's layout

#### Scenario: Unknown template rejected

- **WHEN** a user sets a `template_id` that is not in the registry
- **THEN** the update is rejected with a client error and no CV is rendered with an unknown template

## ADDED Requirements

### Requirement: Available CV templates are discoverable via the API

The system SHALL expose the registered CV templates over a read endpoint so clients can list the available templates without hard-coding them. Each entry SHALL include the template `id`, a human-facing `label`, a short style descriptor, and an `ats_safe` boolean indicating whether the template follows the ATS single-column contract. The endpoint SHALL be available to any authenticated user allowed to use the CV builder.

#### Scenario: List available templates

- **WHEN** an authorized user requests the CV templates list endpoint
- **THEN** the system returns every registered template with its `id`, `label`, style descriptor, and `ats_safe` flag, including `classic-ats` marked as ATS-safe and `sidebar` marked as not ATS-safe

### Requirement: A CV's template can be set independently of its document

The system SHALL provide an endpoint to change only a CV's `template_id` without re-sending the CV document. The endpoint SHALL be owner-scoped (a foreign or missing CV id is a 404), SHALL reject an unknown `template_id` with a client error, and SHALL leave the CV's title and document unchanged.

#### Scenario: Switch template without touching the document

- **WHEN** an authenticated user sends a valid registered `template_id` to the set-template endpoint for a CV they own
- **THEN** the CV's `template_id` is updated, its title and document are unchanged, and a subsequent PDF render uses the new template

#### Scenario: Unknown template rejected on set

- **WHEN** a user sends a `template_id` that is not in the registry to the set-template endpoint
- **THEN** the request is rejected with a client error and the CV's `template_id` is unchanged

### Requirement: Templates are chosen from a visual gallery

The tailoring artifact panel SHALL present the available templates as a gallery of preview thumbnails, one static preview image per registered template. The gallery SHALL indicate the currently selected template, and selecting a thumbnail SHALL persist the choice and re-render the CV PDF preview with the chosen template. Templates that are not ATS-safe SHALL be visually indicated as such. Every registered template SHALL have a committed preview image so the gallery has no missing thumbnails.

#### Scenario: Selecting a template from the gallery updates the CV

- **WHEN** a user opens the Templates tab and clicks a template thumbnail different from the current one
- **THEN** the CV's `template_id` is persisted and the PDF preview re-renders with the chosen template

#### Scenario: Every template has a preview thumbnail

- **WHEN** the templates gallery is shown
- **THEN** each registered template (`classic-ats`, `centered`, `modern-sans`, `sidebar`) displays its own preview image with no missing thumbnails

#### Scenario: Non-ATS-safe template is indicated

- **WHEN** the gallery lists a template that is not ATS-safe (e.g. `sidebar`)
- **THEN** that template is shown with an indication that it may not parse cleanly in some ATS
