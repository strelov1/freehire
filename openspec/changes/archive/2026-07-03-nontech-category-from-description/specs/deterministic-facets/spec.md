## MODIFIED Requirements

### Requirement: Seniority is derived from the description when the title is silent

The seniority facet SHALL be derived at ingest from three sources, most
authoritative first: (1) a structured grade supplied by the source adapter, (2)
the title dictionary (`classify.Parse`), and (3) a conservative, intent-anchored
phrase match in the job **description**. Each lower source SHALL fill `seniority`
only when the higher ones resolved nothing. The description detector SHALL use
anchored phrases (not the bare title aliases) so incidental prose does not match,
and SHALL emit nothing when there is no clear grade statement (it never guesses,
and it does not infer a grade from a years-of-experience figure). The category
facet has its own, separate description tier (see "Non-technical category is
derived from the description when the title is silent").

#### Scenario: The source grade beats the title and description

- **WHEN** a job carries a structured `seniority=senior` from the source while its
  title resolves `lead`
- **THEN** the derived `seniority` is `senior` (the structured source signal wins)

#### Scenario: The description fills seniority when the title is silent

- **WHEN** a job carries no structured grade, its title states no grade, and its
  description says "we are looking for a senior engineer"
- **THEN** the derived `seniority` is `senior`

#### Scenario: The title grade beats the description

- **WHEN** a job carries no structured grade, its title resolves to `lead`, and its
  description mentions a senior engineer
- **THEN** the derived `seniority` is `lead` (the title wins; description only fills)

#### Scenario: Incidental prose does not set seniority

- **WHEN** a job's title states no grade and its description contains incidental
  phrases like "senior management", "lead the team", "junior colleagues", or
  "report to the head of product" — but no anchored grade statement
- **THEN** the derived `seniority` is empty

#### Scenario: A years-of-experience figure does not set seniority

- **WHEN** a job's title states no grade and its description only says "5+ years of
  experience required"
- **THEN** the derived `seniority` is empty (no band is inferred)

## ADDED Requirements

### Requirement: Non-technical category is derived from the description when the title is silent

The category facet SHALL be derived at ingest from three sources, most
authoritative first: (1) a structured category supplied by the source adapter, (2)
the title dictionary (`classify.Parse`), and (3) a conservative, intent-anchored
phrase match in the job **description** that resolves ONLY confidently
non-technical categories (`marketing`, `sales`, `support`, `management` — the
members of `enrich.NonTechCategories`). Each lower source SHALL fill `category`
only when the higher ones resolved nothing. The description detector SHALL match
role-statement phrases anchored on whole-word boundaries (never bare words), SHALL
NOT match tech-adjacent titles (e.g. `sales engineer`, `solutions engineer`, and
`engineering`/`product`/`project`/`data` manager forms), and SHALL emit nothing
when no anchored non-technical role statement is present (it never guesses). It
SHALL NOT resolve any technical category from the description — a title-silent
technical job keeps an empty category.

#### Scenario: The description fills a non-tech category when the title is silent

- **WHEN** a job carries no structured category, its title resolves nothing, and its
  description says "we are hiring a sales representative"
- **THEN** the derived `category` is `sales`

#### Scenario: The title category beats the description

- **WHEN** a job's title resolves `backend` and its description mentions a marketing team
- **THEN** the derived `category` is `backend` (the title wins; the description tier only fills an empty category)

#### Scenario: Incidental prose does not set a non-tech category

- **WHEN** a job's title resolves nothing and its description contains incidental
  phrases like "work with our sales team" or "collaborate with our support engineers" —
  but no anchored non-technical role statement
- **THEN** the derived `category` is empty

#### Scenario: A tech-adjacent manager is not mislabeled as management

- **WHEN** a job's title resolves nothing and its description states "we are hiring an engineering manager"
- **THEN** the derived `category` is empty (engineering/product/project/data manager forms are excluded from the non-tech detector)

#### Scenario: The description tier resolves no technical category

- **WHEN** a job's title resolves nothing and its description clearly describes a backend engineering role
- **THEN** the derived `category` is empty (the description detector resolves only non-technical categories)
