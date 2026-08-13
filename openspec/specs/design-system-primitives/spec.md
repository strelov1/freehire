# design-system-primitives Specification

## Purpose

Defines the contract a reusable UI component must meet to live in
`freehire-design-system` rather than as a one-off inside `web/`: it is documented in
Storybook, its documented variants match how the app actually uses it, and it renders
correctly under both the light and dark theme.

## Requirements

### Requirement: Every exported primitive has a Storybook story

Every component exported from `design-system/src/index.ts` SHALL have a corresponding
`*.stories.ts` file under `design-system/src`, filed under the `Primitives/<Name>` title. A
primitive with no story is exported but not discoverable by someone browsing the catalogue
before building a new one-off component.

#### Scenario: A new primitive ships without a story

- **WHEN** a component is added to `src/index.ts`'s export list with no matching
  `*.stories.ts` file
- **THEN** the primitive is incomplete — the story is part of shipping it, not a follow-up

### Requirement: A primitive's stories cover its real call-site usage

A primitive promoted from an existing `web/src` component SHALL have a story for each
visually or behaviorally distinct configuration that component's call sites used at the time
of promotion (for example: every fallback state a broken/missing image can produce, every
size or variant token passed at a call site). A story set that only shows the default
configuration understates what the primitive actually has to support.

#### Scenario: A promoted primitive has more call-site configurations than stories

- **WHEN** `CompanyLogo`'s call sites pass three distinct states (a loading image, a
  monogram fallback, an icon-only fallback with no name) and only one is storied
- **THEN** the promotion is incomplete — the missing states are added as stories before the
  primitive is considered done

### Requirement: A primitive renders correctly in both Storybook themes

Every primitive's stories SHALL render without visual defect (illegible text, invisible
borders, a light-only fallback color) when the Storybook toolbar's theme is set to `dark`, in
addition to the `light` default. A primitive styled only against light-mode assumptions is a
regression the moment a caller renders it inside a dark-mode surface.

#### Scenario: A primitive uses a hard-coded light-mode color

- **WHEN** a story is inspected with the toolbar's theme set to `dark` and a text or fill
  color does not adapt (for example, a literal `#fff` background behind white text)
- **THEN** the primitive is not done — it must use the token set that resolves per-theme
  instead of a literal

#### Scenario: A primitive has no theme-sensitive styling

- **WHEN** a primitive's rendering is identical in both themes because it carries no
  background or text color of its own (for example, an SVG icon that inherits `currentColor`)
- **THEN** the requirement is satisfied trivially — there is nothing to adapt
