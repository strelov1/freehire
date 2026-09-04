## Purpose

Lets a candidate save a personal default appearance (template, typography, margins) for their
CVs, edited from one settings screen, so every new CV they create starts already styled the way
they like instead of from the system's generic defaults.

## ADDED Requirements

### Requirement: A user can read their saved CV appearance defaults

The system SHALL expose a read endpoint that returns the authenticated user's CV appearance
defaults: `template_id`, typography (`font_family`, `font_size`, `line_height`), and margins
(`top`, `right`, `bottom`, `left`). When the user has never saved any, the endpoint SHALL return
the system's hardcoded CV defaults rather than an empty or absent shape, so a client never has to
special-case "no defaults saved yet".

#### Scenario: Read previously saved defaults

- **WHEN** an authenticated user who has saved appearance defaults requests them
- **THEN** the system returns the exact template, typography, and margin values they last saved

#### Scenario: Read defaults before ever saving any

- **WHEN** an authenticated user who has never saved appearance defaults requests them
- **THEN** the system returns the system's standard CV defaults (`classic-ats` template,
  template-default typography, 0.5 inch margins on every side)

### Requirement: A user can save their CV appearance defaults

The system SHALL expose a write endpoint that replaces the authenticated user's CV appearance
defaults with the submitted template, typography, and margin values. The submitted values SHALL
be validated and sanitized with the same rules the CV builder already applies to a CV document's
own template/typography/margins: an unknown `template_id` SHALL be rejected, and out-of-range
typography or margin values SHALL be clamped rather than rejected.

#### Scenario: Save valid defaults

- **WHEN** an authenticated user submits a registered `template_id`, in-range typography, and
  in-range margins to the write endpoint
- **THEN** the system stores them as that user's appearance defaults, replacing any previous
  saved defaults

#### Scenario: Unknown template is rejected

- **WHEN** an authenticated user submits a `template_id` that is not in the template registry
- **THEN** the request is rejected with a client error and no defaults are saved

#### Scenario: Out-of-range values are clamped, not rejected

- **WHEN** an authenticated user submits a margin or typography value outside its allowed range
- **THEN** the system stores the value clamped to the nearest bound, the same way an out-of-range
  value is clamped when saved directly on a CV document

### Requirement: Saved defaults only seed newly created CVs

Saved appearance defaults SHALL be applied only at the moment a new base CV is created (seeded
from the stored résumé, or empty when none exists): the new CV's `template_id`, typography, and
margins start from the user's saved defaults when present, or the system's standard defaults
otherwise. Saving, changing, or clearing appearance defaults after a CV exists SHALL NOT alter
that CV's own template, typography, or margins.

#### Scenario: A new CV starts from saved defaults

- **WHEN** a user who has saved appearance defaults creates a new base CV
- **THEN** the new CV's `template_id`, typography, and margins match the user's saved defaults

#### Scenario: Changing defaults does not affect existing CVs

- **WHEN** a user changes their saved appearance defaults after already creating one or more CVs
- **THEN** the template, typography, and margins of every CV created before the change remain
  exactly as they were

### Requirement: Appearance defaults are edited from a dedicated settings screen

The CV list screen SHALL provide an entry point to a dedicated settings screen for editing CV
appearance defaults, using the same template gallery and typography/margin controls the tailoring
workspace uses for an individual CV. The settings screen SHALL require an explicit save action;
changes SHALL NOT be persisted as the user edits them.

#### Scenario: Reach the settings screen from the CV list

- **WHEN** a user on the CV list screen activates the appearance-defaults entry point
- **THEN** the user is taken to the appearance-defaults settings screen showing their current
  effective defaults

#### Scenario: Unsaved edits are not persisted

- **WHEN** a user changes a control on the appearance-defaults settings screen and navigates away
  without saving
- **THEN** the user's previously saved defaults (or the system defaults, if they had none) are
  unchanged
