## ADDED Requirements

### Requirement: A deterministic rule set derives a job's AI skill-signature archetype

The system SHALL provide `internal/aiarchetype`, a curated deterministic rule
set (mirroring the dict-only doctrine of `internal/classify`,
`internal/roletag`, and `internal/skilltag`) that derives a job's
`ai_archetype` — at most one canonical archetype slug — from its already-
resolved `skills` and `category`. It SHALL only evaluate jobs whose category is
`ai_engineering` or `ml_ai`; every other category SHALL yield no archetype.

The archetypes and their rules, evaluated in this fixed priority order (first
match wins):

1. `rag_app_builder` — `rag` AND (`langchain` OR `langgraph` OR `llamaindex`)
   AND `vector-databases`
2. `agent_builder` — `agentic-ai` AND `prompt-engineering` AND `rag`
3. `cloud_ml_platform_engineer` — `mlops` AND `kubernetes` AND (`pytorch` OR
   `tensorflow`)
4. `ml_trainer_researcher` — (`pytorch` OR `tensorflow`) AND NOT `rag` AND NOT
   `agentic-ai`
5. `fullstack_ai_engineer` — (`react` OR `nodejs`) AND (`llm` OR `openai` OR
   `anthropic`)
6. `devops_infra_engineer` — `terraform` AND `kubernetes` AND `docker` AND
   `ci-cd`

It SHALL never guess: a job whose skills and category satisfy none of the six
rules yields no archetype, and a job matching more than one rule's skill
requirements SHALL resolve to the single highest-priority (first) matching
archetype only.

#### Scenario: A job matching the most specific rule resolves to it

- **WHEN** `aiarchetype` derives the archetype for a job in category
  `ai_engineering` whose skills include `rag`, `langchain`, `langgraph`, and
  `vector-databases`
- **THEN** the derived archetype is `rag_app_builder`

#### Scenario: Priority order resolves overlapping matches to the higher-priority archetype

- **WHEN** a job's skills satisfy both the `rag_app_builder` rule and the
  `agent_builder` rule
- **THEN** the derived archetype is `rag_app_builder`, not `agent_builder`

#### Scenario: A job outside the AI/ML category yields no archetype

- **WHEN** `aiarchetype` derives the archetype for a job whose category is
  `backend`
- **THEN** no archetype is derived, regardless of its skills

#### Scenario: No rule matches yields no archetype

- **WHEN** a job's category is `ai_engineering` but its skills satisfy none of
  the six rules
- **THEN** no archetype is derived

### Requirement: The archetype is derived at index time, not stored or backfilled

The `ai_archetype` facet SHALL be computed at index time by `search.FromJob`
from the job's already-resolved `skills` and `category`. There SHALL be no
`jobs.ai_archetype` column, no schema migration, and no `backfill-derive`
support for it; a reindex SHALL populate `ai_archetype` on existing documents
(the same index-only pattern `roletag`'s `roles` facet uses).
`ai_archetype` is an index/search concern and SHALL NOT be added to the public
job wire shape returned by the job read endpoints.

#### Scenario: A reindex populates the archetype without a schema change

- **WHEN** the jobs index is rebuilt for existing jobs in category
  `ai_engineering` or `ml_ai`
- **THEN** each matching document carries an `ai_archetype` value derived from
  its skills and category, and no Postgres column or backfill was required

#### Scenario: The archetype does not appear in the public job read shape

- **WHEN** a job is read through a public job read endpoint (list, detail,
  company, or search result)
- **THEN** the returned job wire shape does not include an `ai_archetype`
  field

### Requirement: The archetype is a filterable search facet

`ai_archetype` SHALL be a filterable Meilisearch attribute, queryable by the
public param name `ai_archetype`, following the same wiring as the `roles`
facet's `role` param.

#### Scenario: Jobs can be filtered by archetype

- **WHEN** a client requests `GET /api/v1/jobs/search?ai_archetype=rag_app_builder`
- **THEN** the results include only jobs whose derived archetype is
  `rag_app_builder`
