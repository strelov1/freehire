## ADDED Requirements

### Requirement: The six archetype values are exposed as a generated frontend enum

The six `ai_archetype` slugs SHALL be exposed as `vocab.AIArchetypeValues` (a
literal Go string slice), consumed by `cmd/gen-contracts` into the generated
web contracts as `AI_ARCHETYPE_VALUES`, so the frontend's valid-value list is
generated from the same source of truth as `internal/aiarchetype`'s rule
table rather than hand-duplicated. `vocab.AIArchetypeValues` SHALL be
cross-checked by a test against the archetype slugs `internal/aiarchetype`'s
rule table actually derives, so the two cannot silently drift apart.

#### Scenario: The generated enum matches the rule table

- **WHEN** `cmd/gen-contracts` emits `AI_ARCHETYPE_VALUES`
- **THEN** it contains exactly the six archetype slugs `internal/aiarchetype`
  can derive: `rag_app_builder`, `agent_builder`,
  `cloud_ml_platform_engineer`, `ml_trainer_researcher`,
  `fullstack_ai_engineer`, `devops_infra_engineer`

#### Scenario: A drift between the vocab list and the rule table fails a test

- **WHEN** `vocab.AIArchetypeValues` and `internal/aiarchetype`'s rule-table
  archetype slugs disagree (a slug added to one but not the other)
- **THEN** the cross-check test fails, naming the mismatch
