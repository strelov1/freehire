## ADDED Requirements

### Requirement: Tailor prompt states template-owned section headings and projects placement

The CV-tailoring system prompt SHALL tell the model that rendered section titles
(Experience, Projects, Education, and similar) are produced by the document
template whenever the corresponding arrays are non-empty, and that the agent
MUST NOT invent heading, title, or section-label fields in CV JSON or via
`cv_edit`. The prompt SHALL also tell the model that portfolio, personal, and
side-project work belongs under `projects[]` (paths like `projects[i].…`), not
under `experience[]`, and that an empty Projects section on the rendered page
means `projects` is empty or entries lack a usable name — not that a heading
must be added.

The `cv_edit` tool description SHALL reinforce the same placement rule so a
model that relies on the schema alone still routes portfolio work correctly.

#### Scenario: Tailor prompt forbids inventing section headings

- **WHEN** a CV-tailoring session is built
- **THEN** its system prompt states that section headings are template-owned and must not be invented as document fields

#### Scenario: Tailor prompt routes portfolio work to projects

- **WHEN** a CV-tailoring session is built
- **THEN** its system prompt states that portfolio and side-project entries belong in `projects[]`, not `experience[]`

#### Scenario: cv_edit description mentions projects vs experience

- **WHEN** the CV tools for a tailoring session are registered
- **THEN** the `cv_edit` tool description mentions editing `projects[i].…` for portfolio work and does not imply inventing a Projects heading field
