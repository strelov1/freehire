## MODIFIED Requirements

### Requirement: Create a CV seeded from the stored résumé

When creating a CV, the system SHALL optionally seed its content from the user's existing `resume_structured` extraction. When no structured résumé is available, the system SHALL create a valid empty skeleton document. Seeding SHALL NOT modify the stored résumé or any analysis. A newly created base CV's `template_id`, typography, and margins SHALL start from the user's saved CV appearance defaults when the user has any, falling back to the system's standard defaults otherwise; an explicit `template_id` given on the create request SHALL still take priority over a saved default.

#### Scenario: Seed from structured résumé

- **WHEN** an authenticated user with a `resume_structured` extraction issues `POST /api/v1/me/cvs` with seeding requested
- **THEN** the new CV's `Document` is pre-filled from the structured résumé (contacts, summary, experience, education, languages, links)

#### Scenario: Create empty CV when no résumé exists

- **WHEN** an authenticated user with no `resume_structured` creates a CV
- **THEN** the system returns a valid empty-skeleton `Document` without error

#### Scenario: New CV inherits saved appearance defaults

- **WHEN** a user who has saved CV appearance defaults creates a new base CV without specifying a `template_id`
- **THEN** the new CV's `template_id`, typography, and margins match the user's saved appearance defaults

#### Scenario: Explicit template on create still wins

- **WHEN** a user who has saved CV appearance defaults creates a new base CV while explicitly specifying a `template_id` in the request
- **THEN** the new CV uses the explicitly requested `template_id`, while its typography and margins still come from the user's saved appearance defaults

### Requirement: Template selection with an extensible registry

Each CV SHALL reference a template by `template_id`, defaulting to the user's saved CV appearance-default template when the creating user has one, or otherwise to the ATS template `classic-ats`. The system SHALL resolve `template_id` through a template registry that carries multiple templates so additional templates can be added without schema changes. Each registered template SHALL have a stable id and a matching Typst template file. An unknown `template_id` SHALL be rejected rather than rendered.

#### Scenario: Default template applied

- **WHEN** a CV is created without an explicit template, by a user with no saved appearance defaults
- **THEN** its `template_id` defaults to `classic-ats` and renders with that template

#### Scenario: A non-default registered template renders

- **WHEN** a user sets a CV's `template_id` to another registered template (e.g. `centered`, `modern-sans`, or `sidebar`)
- **THEN** the CV is accepted and rendered with that template's layout

#### Scenario: Unknown template rejected

- **WHEN** a user sets a `template_id` that is not in the registry
- **THEN** the update is rejected with a client error and no CV is rendered with an unknown template

### Requirement: CV documents carry configurable page margins

A CV `Document` SHALL carry four page-margin values — top, right, bottom, and left — expressed
in inches. Margins are part of the document (not separate metadata), so they persist through the
existing CV storage, are copied when a CV is tailored, and are not clobbered by field-level
patches. On an existing document, any margin the user has not set defaults to 0.5 inch. On a
newly created base CV, the starting margins come from the creating user's saved CV appearance
defaults when they have any, and from the 0.5 inch system default otherwise.

#### Scenario: Unset margins default to half an inch

- **WHEN** a CV document is persisted with no margin values (or a zero value on a side)
- **THEN** that side's margin is treated as 0.5 inch

#### Scenario: Margins persist with the document

- **WHEN** a CV is saved with margins {top: 0.75, right: 0.5, bottom: 0.75, left: 0.5}
- **THEN** re-reading the CV returns those same margin values

#### Scenario: New CV starts from saved default margins

- **WHEN** a user who has saved CV appearance defaults with margins {top: 0.75, right: 0.5, bottom: 0.75, left: 0.5} creates a new base CV
- **THEN** the new CV's margins are {top: 0.75, right: 0.5, bottom: 0.75, left: 0.5}, not the 0.5 inch system default
