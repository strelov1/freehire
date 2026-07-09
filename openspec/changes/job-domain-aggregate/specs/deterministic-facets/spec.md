## MODIFIED Requirements

### Requirement: The six dictionary facets are sourced solely from the deterministic dictionaries

The system SHALL store the deterministic, dictionary-derived facets —
`countries`, `regions`, `work_mode`, `skills`, `seniority`, `category` — in the
`jobs` table columns as source facts, computed by the curated dictionaries
(`jobderive`, wrapping `location`/`skilltag`/`classify`), and SHALL derive them
through the `Job` aggregate factory (`job.New`) on every write path, so the
derivation cannot diverge between the ingest, moderator-authoring, and Telegram
paths. The public read model (`jobview`, projecting from the domain `Job` via
`jobview.FromDomain`) SHALL source these six facets from the `jobs` columns ONLY.
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

#### Scenario: The Telegram path derives facets identically to ingest

- **WHEN** a Telegram-extracted posting and a board-ingested posting carry the same
  title, description, and location
- **THEN** they resolve the same dictionary facets, because both construct their
  `Job` through the aggregate factory rather than deriving inline
