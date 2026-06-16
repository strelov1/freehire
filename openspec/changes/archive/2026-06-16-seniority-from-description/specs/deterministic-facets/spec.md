## ADDED Requirements

### Requirement: Seniority is derived from the description when the title is silent

The seniority facet SHALL be derived at ingest from two sources, most
authoritative first: (1) the title dictionary (`classify.Parse`), and (2) a
conservative, intent-anchored phrase match in the job **description**. The
description SHALL fill `seniority` only when the title dictionary resolved
nothing. The description detector SHALL use anchored phrases (not the bare title
aliases) so incidental prose does not match, and SHALL emit nothing when there is
no clear grade statement (it never guesses, and it does not infer a grade from a
years-of-experience figure). The category facet is unaffected by this requirement.

#### Scenario: The description fills seniority when the title is silent

- **WHEN** a job's title states no grade and its description says "we are looking
  for a senior engineer"
- **THEN** the derived `seniority` is `senior`

#### Scenario: The title grade beats the description

- **WHEN** a job's title resolves to `lead` and its description mentions a senior
  engineer
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
