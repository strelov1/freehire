## ADDED Requirements

### Requirement: CV documents carry typography settings

A CV `Document` SHALL carry a `style` block with three optional typography values — font family,
base font size in points, and line height as a leading multiple of the em. Typography is part of the
document (not separate metadata), so it persists through the existing CV storage, is copied when a
CV is tailored, and is not clobbered by field-level patches.

An unset value SHALL mean "use the active template's own", and SHALL be preserved as unset rather
than resolved to a concrete value on persist. A template therefore remains a complete design choice:
switching templates changes any typography the candidate has not overridden.

#### Scenario: An unset style renders as the template defines it

- **WHEN** a CV document with no `style` values is rendered
- **THEN** it renders with the template's own typeface, type size, and leading, identically to how it
  rendered before typography settings existed

#### Scenario: Unset values survive a round trip

- **WHEN** a CV is saved with a font size set but no font family and no line height
- **THEN** re-reading the CV returns the font size and returns the family and line height still unset

#### Scenario: Typography persists with the document

- **WHEN** a CV is saved with font family `liberation-serif`, font size 10.5, and line height 0.6
- **THEN** re-reading the CV returns those same three values

#### Scenario: Switching templates moves the values the candidate has not set

- **WHEN** a CV that sets only a font size is switched to a template whose leading differs
- **THEN** it renders with the candidate's font size and the new template's leading

### Requirement: Typography settings are sanitized on persist

The CV sanitizer SHALL clamp a set font size to the range 8.5–12.0 points, rounded to the nearest
0.5, and a set line height to the range 0.3–0.9 em, so a persisted CV never carries a type size that
is unreadable or a leading that collapses lines into each other.

A font family that is not in the font registry SHALL be reset to unset rather than rejected. This
follows the sanitizer's existing contract — it repairs a document rather than failing it, the same
way an out-of-range margin is clamped instead of refused.

#### Scenario: An out-of-range font size is clamped

- **WHEN** a CV document is persisted with a font size of 30.0 or 4.0
- **THEN** the stored font size is clamped to 12.0 (upper bound) or 8.5 (lower bound) respectively

#### Scenario: A font size is rounded to the nearest half point

- **WHEN** a CV document is persisted with a font size of 10.3
- **THEN** the stored font size is 10.5

#### Scenario: An out-of-range line height is clamped

- **WHEN** a CV document is persisted with a line height of 2.0 or 0.05
- **THEN** the stored line height is clamped to 0.9 (upper bound) or 0.3 (lower bound) respectively

#### Scenario: An unregistered font family is dropped

- **WHEN** a CV document is persisted with a font family that is not in the registry
- **THEN** the stored font family is unset and the CV renders with the template's own typeface

#### Scenario: A zero value is left alone

- **WHEN** a CV document is persisted with a zero font size and a zero line height
- **THEN** both stay zero and are not clamped up to the lower bound, because zero means "inherit"

### Requirement: Fonts are chosen from an extensible registry

The system SHALL resolve a document's font family through a font registry so additional typefaces can
be added without a schema change. Each registered font SHALL have a stable id, a human-facing label,
and a resolvable face available to the renderer without relying on the host's installed fonts. The
registry SHALL include typefaces that are metric-compatible with the faces résumé-scanning software
and recruiters expect, and each SHALL be redistributable under an open font licence carried in the
repository.

#### Scenario: A registered font renders

- **WHEN** a CV sets its font family to a registered id and is rendered
- **THEN** the PDF is typeset in that face and its text layer remains extractable

#### Scenario: Rendering does not depend on host fonts

- **WHEN** a CV using any registered font is rendered on a machine with no system fonts installed
- **THEN** the requested face is still used, because it is bundled with the application rather than
  looked up on the host

### Requirement: Available CV fonts are discoverable via the API

The system SHALL expose the registered CV fonts over a read endpoint so clients list the available
typefaces without hard-coding them. Each entry SHALL include the font `id`, a human-facing `label`,
and a short note naming the familiar face it matches where one applies. The endpoint SHALL be
available to any authenticated user allowed to use the CV builder.

#### Scenario: List available fonts

- **WHEN** an authorized user requests the CV fonts list endpoint
- **THEN** the system returns every registered font with its `id`, `label`, and note

### Requirement: Rendering honours the document's typography

The renderer SHALL apply the document's font family, base font size, and line height, and the
template's own type hierarchy SHALL scale with the base size rather than being fixed against it — a
larger base size SHALL keep the candidate's name and section headings proportionally larger than
body text.

The document SHALL NOT carry rendering-engine-specific font names: the renderer resolves a registry
id to whatever the engine needs, on a copy, leaving the stored document untouched.

#### Scenario: A raised base size scales the hierarchy

- **WHEN** a CV is rendered at a base font size of 12.0
- **THEN** the candidate's name and section headings are still larger than the body text

#### Scenario: Every template honours the style block

- **WHEN** a CV with a set font family, font size, and line height is rendered with each registered
  template in turn
- **THEN** every template renders with those three values applied

#### Scenario: Resolving a font leaves the stored document alone

- **WHEN** a CV is rendered
- **THEN** the stored document still holds the registry id, not the renderer's internal face name
