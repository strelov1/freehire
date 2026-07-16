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

The system SHALL render a CV to PDF on request and stream it as `application/pdf` without persisting the file. The rendered PDF SHALL contain a selectable text layer (not a rasterized image) and follow the ATS template contract: single column, standard section headings, embedded fonts without problematic ligatures.

#### Scenario: Download a CV PDF

- **WHEN** an authenticated user requests `GET /api/v1/me/cvs/:id/pdf` for a CV they own
- **THEN** the system renders the CV via the configured renderer and streams a `application/pdf` response

#### Scenario: Rendered text is extractable

- **WHEN** the CV PDF is rendered and its text layer is extracted
- **THEN** the candidate's name and skills appear as selectable text in the extracted output

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

Each CV SHALL reference a template by `template_id`, defaulting to the single shipped ATS template (`classic-ats`). The system SHALL resolve `template_id` through a template registry so additional templates can be added without schema changes. An unknown `template_id` SHALL be rejected rather than rendered.

#### Scenario: Default template applied

- **WHEN** a CV is created without an explicit template
- **THEN** its `template_id` defaults to `classic-ats` and renders with that template

#### Scenario: Unknown template rejected

- **WHEN** a user sets a `template_id` that is not in the registry
- **THEN** the update is rejected with a client error and no CV is rendered with an unknown template

