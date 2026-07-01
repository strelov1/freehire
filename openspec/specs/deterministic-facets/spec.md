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

### Requirement: The category facet distinguishes AI engineering from classic ML

The `category` vocabulary SHALL include `ai_engineering` alongside `ml_ai`, and
the title dictionary SHALL route role titles between them: titles denoting
LLM/GenAI application work (RAG, agents, prompt engineering, applied AI, "AI
Engineer") resolve to `ai_engineering`, while titles denoting classic machine
learning / deep learning model work resolve to `ml_ai`. A title that explicitly
carries the ML token in a combined form (`ML/AI`, `AI/ML`, `ML Engineer`) SHALL
resolve to `ml_ai` — the ML signal wins over the bare AI signal. The dictionary
never guesses: a title naming neither resolves to an empty category, unchanged.

#### Scenario: An AI-application title resolves to ai_engineering

- **WHEN** a job titled "Applied AI Engineer" (or "LLM Engineer",
  "Generative AI Engineer", "Prompt Engineer", "RAG Engineer", "AI Engineer") is
  classified by the title dictionary
- **THEN** the derived `category` is `ai_engineering`

#### Scenario: A classic-ML title resolves to ml_ai

- **WHEN** a job titled "Machine Learning Engineer" (or "Deep Learning Engineer",
  "ML Engineer") is classified by the title dictionary
- **THEN** the derived `category` is `ml_ai`

#### Scenario: A combined ML-carrying title stays ml_ai

- **WHEN** a job titled "ML/AI Engineer" or "AI/ML Engineer" is classified by the
  title dictionary
- **THEN** the derived `category` is `ml_ai` (the explicit ML token wins over the
  bare AI token)

#### Scenario: ai_engineering is a recognized category value

- **WHEN** the LLM enrichment payload or any consumer validates a `category` of
  `ai_engineering` against the controlled vocabulary
- **THEN** the value is accepted (it is a member of the category vocabulary), not
  sanitized away

