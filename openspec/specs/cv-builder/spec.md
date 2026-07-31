# cv-builder Specification

## Purpose
TBD - created by archiving change add-cv-builder. Update Purpose after archive.
## Requirements
### Requirement: CV builder is restricted to beta testers

The CV-builder feature SHALL be a restricted rollout: every `/api/v1/me/cvs` endpoint SHALL require an authenticated caller who is a beta tester (or a moderator). A signed-in caller who is neither SHALL receive `403`. The SPA SHALL only surface the CV-builder navigation and pages to eligible users.

#### Scenario: Non-beta user is forbidden

- **WHEN** a signed-in user who is neither a beta tester nor a moderator calls any `/api/v1/me/cvs` endpoint
- **THEN** the system returns `403` and performs no CV operation

#### Scenario: Beta tester has access

- **WHEN** a signed-in beta tester (or moderator) calls a `/api/v1/me/cvs` endpoint
- **THEN** the request is authorized and proceeds

### Requirement: Users own multiple structured CVs

The system SHALL let an authenticated user create, list, read, update, and delete multiple CVs. Each CV SHALL be a structured document (typed sections: header, summary, experience, education, skills, languages, projects, certifications) stored as JSON, with metadata (`title`, `template_id`, timestamps). CVs SHALL be scoped to their owner and never exposed to other users.

#### Scenario: List own CVs

- **WHEN** an authenticated user requests `GET /api/v1/me/cvs`
- **THEN** the system returns only that user's CVs as metadata (id, title, template_id, updated_at), newest first

#### Scenario: Read a single CV

- **WHEN** an authenticated user requests `GET /api/v1/me/cvs/:id` for a CV they own
- **THEN** the system returns the full structured `Document`

#### Scenario: Reject access to another user's CV

- **WHEN** an authenticated user requests, updates, or deletes a CV id owned by a different user
- **THEN** the system returns `404` and does not reveal the CV

#### Scenario: Delete a CV

- **WHEN** an authenticated user issues `DELETE /api/v1/me/cvs/:id` for a CV they own
- **THEN** the CV is removed and subsequent reads of that id return `404`

### Requirement: Create a CV seeded from the stored résumé

When creating a CV, the system SHALL optionally seed its content from the user's existing `resume_structured` extraction. When no structured résumé is available, the system SHALL create a valid empty skeleton document. Seeding SHALL NOT modify the stored résumé or any analysis.

#### Scenario: Seed from structured résumé

- **WHEN** an authenticated user with a `resume_structured` extraction issues `POST /api/v1/me/cvs` with seeding requested
- **THEN** the new CV's `Document` is pre-filled from the structured résumé (contacts, summary, experience, education, languages, links)

#### Scenario: Create empty CV when no résumé exists

- **WHEN** an authenticated user with no `resume_structured` creates a CV
- **THEN** the system returns a valid empty-skeleton `Document` without error

### Requirement: CV content is sanitized on persist

The system SHALL sanitize every CV `Document` before persisting it: bound each string length, cap array sizes, and drop out-of-range values. Sanitization SHALL serve as both the persistence guard and the prompt-injection guard for untrusted CV text.

#### Scenario: Oversized input is bounded

- **WHEN** a user submits a `Document` with over-long strings or over-large arrays via `PUT /api/v1/me/cvs/:id`
- **THEN** the system stores the sanitized (bounded, capped) document, never the raw oversized input

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

### Requirement: Photo-bearing templates compose the stored headshot

The registry SHALL mark which templates print a headshot, and rendering such a template SHALL compose the owner's stored headshot into the PDF. The image SHALL reach the renderer as bytes staged alongside the document, not as a URL the rendering engine fetches, so rendering stays sandboxed and makes no outbound request. A template not marked as photo-bearing SHALL render identically to before, and the system SHALL NOT read object storage for it.

#### Scenario: A photo-bearing template prints the headshot

- **WHEN** a user with a stored headshot renders a CV whose template is photo-bearing
- **THEN** the PDF contains that image, and the CV's text layer remains extractable

#### Scenario: A photoless template costs no storage read

- **WHEN** a CV using a template that is not photo-bearing is rendered
- **THEN** the headshot is not fetched from object storage and the output is unchanged from a render made without any stored headshot

### Requirement: A photo-bearing template without a headshot renders a placeholder

The system SHALL render a neutral silhouette placeholder in the photo frame when a photo-bearing template is used by a candidate who has no stored headshot, rather than failing the render or leaving an empty gap. The placeholder SHALL be drawn by the template itself, so it needs no stored asset and appears identically in the generated gallery thumbnails.

#### Scenario: No headshot uploaded

- **WHEN** a user with no stored headshot renders a CV using a photo-bearing template
- **THEN** the render succeeds and the photo frame shows the silhouette placeholder

#### Scenario: Gallery thumbnail shows the photo frame

- **WHEN** the committed preview thumbnails are regenerated from the sample document, which carries no headshot
- **THEN** each photo-bearing template's thumbnail shows its photo frame with the placeholder, so the gallery represents the template's actual layout

### Requirement: Rendering is behind a swappable interface

The system SHALL render through a `Renderer` interface so the CV schema, storage, and API do not depend on the concrete rendering engine. The default implementation SHALL use Typst. Replacing or adding a renderer SHALL NOT require changes to the CV schema, storage, or handlers.

#### Scenario: Engine isolated from data model

- **WHEN** the rendering engine is swapped for another `Renderer` implementation
- **THEN** the CV `Document` schema, `cvs` storage, and CRUD endpoints remain unchanged

### Requirement: PDF rendering is feature-gated and degrades gracefully

The system SHALL treat the renderer as optional. When the renderer is not configured (no Typst binary available), CV create/read/update/delete SHALL keep working and the PDF endpoint SHALL return `501 Not Implemented`.

#### Scenario: Renderer not configured

- **WHEN** no Typst binary is configured and a user requests `GET /api/v1/me/cvs/:id/pdf`
- **THEN** the system returns `501` while all non-PDF CV endpoints continue to work

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

### Requirement: Available CV templates are discoverable via the API

The system SHALL expose the registered CV templates over a read endpoint so clients can list the available templates without hard-coding them. Each entry SHALL include the template `id`, a human-facing `label`, a short style descriptor, an `ats_safe` boolean indicating whether the template follows the ATS single-column contract, and a `photo` boolean indicating whether the template prints a headshot. The endpoint SHALL be available to any authenticated user allowed to use the CV builder.

#### Scenario: List available templates

- **WHEN** an authorized user requests the CV templates list endpoint
- **THEN** the system returns every registered template with its `id`, `label`, style descriptor, `ats_safe` flag, and `photo` flag, including `classic-ats` marked as ATS-safe and photoless, and `sidebar` marked as not ATS-safe

#### Scenario: Photo-bearing templates are identifiable

- **WHEN** an authorized user lists the CV templates
- **THEN** the photo-bearing templates are returned with `photo` true and `ats_safe` false, so a client can prompt for a headshot upload before the template is used

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

### Requirement: A CV id is unguessable

A CV SHALL be identified by a random id rather than by a sequential one, in the
database and on every surface that names it — API paths, web routes, and the
published clients. Ownership already confines every read and write, so the id is
not a capability; but a countable id would publish how many CVs the platform
holds, and it would turn a single missing owner check on any future CV endpoint
into bulk extraction of other people's résumés. An id that is not well-formed
SHALL be reported as a missing CV, so "not a CV" and "not yours" stay one answer.

#### Scenario: Two CVs get unrelated ids

- **WHEN** a user creates two CVs in a row
- **THEN** their ids are independently random, so neither reveals the other nor how many CVs exist

#### Scenario: A malformed id is missing, not invalid

- **WHEN** a request names a CV id that is not well-formed — a number, or anything that is not an id at all
- **THEN** it is refused as not found, indistinguishable from a CV the caller does not own

#### Scenario: The numeric form no longer resolves

- **WHEN** a client built against the previous numeric ids sends one
- **THEN** the request is refused as not found rather than resolving to any CV
