## ADDED Requirements

### Requirement: A facet code renders as one string on every surface

The SPA SHALL resolve a closed-vocabulary facet code to its display text through a single
shared label map, so the filter panel, the job-detail facet rows, and the indexed
`/insights` pages cannot render the same code under different names. A surface MUST NOT
declare its own map for a vocabulary the shared module already owns, and MUST NOT reach a
shared map through a fallback of its own.

#### Scenario: The same category on three surfaces

- **WHEN** a reader sees the category `network_engineering` in the filter panel, in the
  Category row of a job's detail page, and in the H1 of `/insights/roles/network_engineering`
- **THEN** all three read "Network Engineering"

#### Scenario: The same relocation value in a filter and on a job

- **WHEN** a reader sees the relocation value `not_supported` as a filter option and as the
  Relocation row of a job's detail page
- **THEN** both read "Not supported"

### Requirement: The category vocabulary is labelled exhaustively

The shared category label map SHALL carry an entry for every value in the generated
`CATEGORY_VALUES` vocabulary, so no category is ever rendered through a fallback. The
binding between map and vocabulary SHALL be enforced by an automated check rather than by
convention, so adding a category to the backend vocabulary fails the suite until the SPA is
given its display text.

#### Scenario: Every generated category has a label

- **WHEN** the label suite runs
- **THEN** it asserts that each value of `CATEGORY_VALUES` is a key of the shared category map

#### Scenario: A new backend category arrives without a label

- **WHEN** a category is added to the backend vocabulary and the generated contracts are
  regenerated, but no display text is added
- **THEN** the label suite fails, naming the unlabelled code

### Requirement: An unrecognised code still renders as readable text

The label lookup SHALL fall back to a title-cased rendering of the code for any value outside
the map, so a vocabulary the SPA has not yet been taught renders as readable text rather than
as a blank, a raw snake_case token, or an error. This fallback is a safety net for
unrecognised input, not the mechanism by which known codes are labelled.

#### Scenario: A code the SPA has never seen

- **WHEN** the SPA is asked to render the category code `quantum_widgets`, which is in no map
- **THEN** it renders "Quantum Widgets"

### Requirement: The settled wordings

The shared maps SHALL carry these display strings, which resolve wordings that previously
differed between surfaces:

- `ai_engineering` renders "AI Engineering" — the vocabulary names disciplines
  (Data Engineering, Network Engineering, Project Management), not job titles.
- `fullstack` renders "Full-Stack" — already the text on the indexed `/insights` page, and
  consistent with the hyphenated compounds elsewhere in the vocabulary (Full-time, On-site,
  C-level, In-house).
- relocation `not_supported` renders "Not supported" — grammatically parallel to its sibling
  values Supported and Required, where the former filter-panel wording "None" was ambiguous
  with "not stated".

#### Scenario: AI engineering is a discipline

- **WHEN** the code `ai_engineering` is rendered on any surface
- **THEN** it reads "AI Engineering", not "AI Engineer"

#### Scenario: Full-stack keeps its indexed spelling

- **WHEN** the code `fullstack` is rendered on any surface
- **THEN** it reads "Full-Stack", the spelling already published at `/insights/roles/fullstack`

### Requirement: The `/insights` pages keep their published category set and copy

The `/insights` capability SHALL keep deciding which categories are published and how its
intro sentences read; only the source of the category's display text changes. The
category-wide seniority band, which is not a value of the seniority vocabulary, SHALL keep
its own label on those pages.

#### Scenario: The all-levels band

- **WHEN** an `/insights` salary table renders the category-wide band, whose seniority value
  is the empty string
- **THEN** it reads "All levels", and the shared seniority map is not consulted for it

#### Scenario: The published set is unchanged

- **WHEN** the change is applied
- **THEN** the set of categories that clear the publication floor, and the auto-intro
  sentences generated for them, are exactly what they were before
