# deterministic-facets Specification

## Purpose
TBD - created by archiving change dict-production-facets. Update Purpose after archive.
## Requirements
### Requirement: The six dictionary facets are sourced solely from the deterministic dictionaries

The system SHALL store the deterministic, dictionary-derived facets —
`countries`, `regions`, `work_mode`, `skills`, `seniority`, `category` — in the
`jobs` table columns as source facts, computed by the curated dictionaries
(`jobderive`, wrapping `location`/`skilltag`/`classify`). The public read model
(`jobview.FromRow`) SHALL source these six facets from the `jobs` columns ONLY.
It SHALL NOT union the multi-valued facets (`countries`/`regions`/`skills`) with
their `enrichment` counterparts, and it SHALL NOT let the LLM-derived
`enrichment.work_mode`/`enrichment.seniority`/`enrichment.category` override or
fall back into the served scalar facets. The dictionaries emit nothing for what
they cannot resolve, so an unresolved facet is served empty rather than filled
from the LLM.

#### Scenario: A multi-valued facet drops the LLM contribution

- **WHEN** a job has `skills=[go]` from the dictionary and
  `enrichment.skills=[go, kubernetes]` from the LLM, and is read
- **THEN** the served `skills` is `[go]` (the LLM's `kubernetes` is not unioned in)

#### Scenario: A scalar facet is the dictionary value, never the LLM's

- **WHEN** a job has `seniority=middle` from the title dictionary and
  `enrichment.seniority=senior` from the LLM, and is read
- **THEN** the served `seniority` is `middle`

#### Scenario: A dictionary-silent facet is served empty, not from the LLM

- **WHEN** a job has an empty `category` column (the title dictionary resolved
  nothing) and `enrichment.category=backend` from the LLM, and is read
- **THEN** the served `category` is empty

### Requirement: Raw LLM facet values are retained but not served

The change SHALL leave the stored `jobs.enrichment` JSONB untouched: the LLM's
values for the six dictionary facets remain persisted as raw material for a later
discovery workflow. Only the served wire shape excludes them; the database is not
rewritten to drop them.

#### Scenario: Enrichment JSONB keeps the LLM facet values after a read

- **WHEN** a job whose `enrichment` JSONB contains `regions`, `work_mode`,
  `seniority`, and `skills` is read through the public model
- **THEN** the served object omits those LLM values from the six facets
- **AND** the stored `enrichment` JSONB still contains them unchanged

### Requirement: Existing jobs are re-derived by a single unified backfill

The system SHALL provide one run-once command that re-derives all six dictionary
facet columns (`countries`, `regions`, `work_mode`, `skills`, `seniority`,
`category`) for existing jobs in a single pass by calling `jobderive.Derive`,
replacing the three separate per-facet backfill commands. The pass SHALL rewrite
only those facet columns and SHALL NOT touch the slugs (re-slugging stays a
distinct command). It SHALL be idempotent — re-running converges to the same
result — and SHALL include closed jobs (which never re-crawl). To preserve a
structured ATS work-mode signal not available at backfill time, `work_mode` SHALL
be filled from the parsed location only when the row's `work_mode` is empty.

#### Scenario: One pass rewrites all six facet columns

- **WHEN** the backfill runs over a job with a resolvable title, location, and
  description whose facet columns are stale or empty
- **THEN** the job's `countries`, `regions`, `work_mode`, `skills`, `seniority`,
  and `category` columns are all rewritten from the dictionaries in that one pass
- **AND** the job's `public_slug` and `company_slug` are unchanged

#### Scenario: The unified backfill is idempotent

- **WHEN** the backfill is run twice over the same jobs
- **THEN** the second run produces the same facet columns as the first

#### Scenario: A set work mode is preserved when the location cannot improve it

- **WHEN** the backfill runs over a job with `work_mode=hybrid` already set and a
  location that parses to no work-mode hint
- **THEN** the job's `work_mode` stays `hybrid`

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

### Requirement: The skills dictionary covers technologies, not soft skills or domains

The `skills` facet SHALL be derived from a curated **technology** dictionary —
languages, frameworks, datastores, infrastructure, platforms, and engineering
methodologies — and SHALL NOT include soft skills (e.g. communication, leadership)
or industry/domain terms (e.g. retail, nursing), keeping the facet a high-signal
technology filter. The dictionary resolves only known aliases and emits nothing for
what it cannot match (it never guesses).

#### Scenario: A common methodology resolves

- **WHEN** a job description states "we work in an agile environment using scrum"
- **THEN** the derived `skills` include `agile` and `scrum`

#### Scenario: A platform resolves

- **WHEN** a job description mentions "Salesforce and SAP integration experience"
- **THEN** the derived `skills` include `salesforce` and `sap`

#### Scenario: An incidental word does not tag

- **WHEN** a job description says "you will support the rest of the team" but states
  no REST API
- **THEN** the derived `skills` do not include `rest`

