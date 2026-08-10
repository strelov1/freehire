## ADDED Requirements

### Requirement: Candidate visibility setting
A signed-in user SHALL be able to set their Talent Network visibility to one
of `off`, `public`, or `anonymous` from their profile settings. The default
for every user, including existing users, SHALL be `off`.

#### Scenario: Default state for an existing user
- **WHEN** a user who has never touched this setting views their profile
- **THEN** their Talent Network visibility is `off`

#### Scenario: Candidate changes visibility
- **WHEN** a signed-in user selects `public` or `anonymous` on their profile
  settings and saves
- **THEN** the user's Talent Network visibility is updated to the selected
  value and persists across sessions

### Requirement: Public profile page availability
The system SHALL serve an unauthenticated, publicly reachable page for each
user at a stable URL keyed by an opaque identifier (never the sequential
database ID), whose content depends on that user's current visibility
setting.

#### Scenario: Visibility off
- **WHEN** anyone (including a logged-out visitor) requests the page for a
  user whose visibility is `off`
- **THEN** the response is 404

#### Scenario: Nonexistent profile
- **WHEN** anyone requests the page for an opaque identifier that does not
  correspond to any user
- **THEN** the response is 404, identical in shape to the `off` case

#### Scenario: Visibility public
- **WHEN** anyone requests the page for a user whose visibility is `public`
- **THEN** the response is 200 and renders that user's name, work history,
  and skills. (No photo in this slice — see design.md's "Non-Goals": the
  only existing photo-read path is cookie-only, and a public image route is
  separable follow-up scope, not part of this change.)

#### Scenario: Visibility anonymous
- **WHEN** anyone requests the page for a user whose visibility is
  `anonymous`
- **THEN** the response is 200 and renders that user's work history and
  skills, with no name and no photo shown

### Requirement: Contact info never shown on the public page
The public profile page SHALL NOT display the candidate's raw email, phone
number, or any personal links, regardless of visibility mode.

#### Scenario: Public mode omits contact fields
- **WHEN** the public page renders for a user whose visibility is `public`
- **THEN** the rendered content contains no email address, phone number, or
  personal link belonging to that user

#### Scenario: Anonymous mode omits contact fields
- **WHEN** the public page renders for a user whose visibility is
  `anonymous`
- **THEN** the rendered content contains no email address, phone number, or
  personal link belonging to that user

### Requirement: Anonymous mode masks the current employer
In `anonymous` visibility, every work history entry whose end date reads as
"not ended" (empty, or one of `present`/`current`/`now`/`ongoing`,
case-insensitive — the same convention `internal/experience` already uses to
decide this) SHALL have its employer name replaced with a generic label. All
other work history entries SHALL be shown unmodified. This is a content-based
rule, not a positional one — array order is not a reliable signal for which
entry is current.

#### Scenario: Current employer is masked
- **WHEN** the public page renders for a user whose visibility is
  `anonymous` and one work history entry's end date reads as "not ended"
- **THEN** that entry's employer name is replaced with a generic label and
  every other entry's employer name is shown as-is

#### Scenario: Multiple concurrent entries are all masked
- **WHEN** the public page renders for a user whose visibility is
  `anonymous` and more than one work history entry's end date reads as
  "not ended"
- **THEN** every such entry's employer name is replaced with a generic
  label

#### Scenario: No entry reads as current
- **WHEN** the public page renders for a user whose visibility is
  `anonymous` and no work history entry's end date reads as "not ended"
- **THEN** no employer name is masked

### Requirement: Enabling visibility does not require a complete CV
A user SHALL be able to set their visibility to `public` or `anonymous`
regardless of whether they have uploaded or completed a structured CV.

#### Scenario: Visibility enabled with no CV data
- **WHEN** the public page renders for a user whose visibility is `public`
  or `anonymous` and who has no structured CV data
- **THEN** the response is 200 and renders with an empty work history and
  skills section, rather than an error
