## ADDED Requirements

### Requirement: A structured source facet signal takes precedence over the dictionaries

The `seniority`, `category`, `skills`, and `experience_years_min` facets SHALL
accept a structured signal supplied by the source adapter, taking precedence over
the deterministic dictionary, generalizing the rule `work_mode` already follows.
For the scalar facets (`seniority`, `category`, `experience_years_min`), the
source signal SHALL be used when present and the dictionary/description SHALL fill
the facet only when the source is silent. For the multi-valued `skills` facet, the
source signal SHALL be UNIONED with the dictionary-derived skills (both are facts;
neither replaces the other). The structured signal feeds the `jobs` columns at
ingest through `jobderive`; the public read model is unchanged — `jobview.FromRow`
still serves these facets from the `jobs` columns only and never from the LLM.

#### Scenario: A source grade beats the title dictionary

- **WHEN** a job carries a structured `seniority=senior` from the source and its
  title dictionary resolves nothing (or a different grade)
- **THEN** the derived `seniority` is `senior`

#### Scenario: The dictionary fills a facet the source left empty

- **WHEN** a job carries no structured `category` from the source and its title
  dictionary resolves `backend`
- **THEN** the derived `category` is `backend`

#### Scenario: Source skills union with dictionary skills

- **WHEN** a job carries structured `skills=[go]` from the source and the
  description dictionary resolves `skills=[kubernetes]`
- **THEN** the derived `skills` are `[go, kubernetes]`

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
facet is unaffected by this requirement.

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
