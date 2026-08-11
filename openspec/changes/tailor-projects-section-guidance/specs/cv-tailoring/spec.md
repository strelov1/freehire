## ADDED Requirements

### Requirement: Seeded base CVs place portfolio work in projects

When the system seeds or refreshes a base CV from the structured résumé and/or
the experience bank, portfolio and personal projects SHALL land in
`document.projects`, not in `document.experience`. A base CV whose sources
include such projects MUST NOT persist with an empty `projects` array while
those entries exist only under experience or only on a tailored copy.

#### Scenario: Bank project becomes a CV project on seed

- **WHEN** the experience bank holds a `kind=project` employment and a base CV is seeded or refreshed from the bank
- **THEN** that entry appears under `projects` on the base document and not as an `experience` role

#### Scenario: Structure projects become CV projects on seed

- **WHEN** the current structured résumé lists projects and a base CV is seeded from that structure
- **THEN** those entries appear under `projects` on the base document
