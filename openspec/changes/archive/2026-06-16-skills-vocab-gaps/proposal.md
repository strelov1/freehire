## Why

Under the dict-only doctrine, the `skills` facet is served from the `skilltag`
dictionary only. Mining prod (open jobs' `enrichment.skills`) shows the dictionary
is already comprehensive on languages/frameworks/datastores/infra, but misses a
small set of high-frequency **methodologies and platforms** — `agile` (3568),
`salesforce` (3548), `sap` (2447), `microservices` (1842), `devops` (1446),
`observability` (1352), `scrum` (1228), `rest` (1213), `oracle` (1131). These are
genuine tech-skill gaps (the soft-skill and domain terms the LLM also emits —
communication, retail, nursing — are deliberately OUT of scope: `skilltag` is a
technology dictionary, and adding them would dilute the facet with low-signal noise).

## What Changes

- Add to `internal/skilltag` the missing tech/methodology aliases, keeping the
  dictionary's technology focus: `agile`, `scrum`, `kanban`, `salesforce`, `sap`,
  `oracle`, `devops`, `microservices` (+ `microservice`), `observability` (word
  pass); `rest api`/`rest apis`/`restful` → `rest`, and `power bi`/`power-bi` →
  `powerbi` (phrase pass). **Bare `rest` is deliberately excluded** (it matches "the
  rest of the team").
- No engine change, no schema, no new command: `cmd/backfill-derive` re-derives
  through `skilltag`, so a post-deploy backfill recovers existing jobs.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

(none — this is a pure vocabulary expansion; the "skills are derived from a curated
dictionary" requirement is unchanged, only its data grows.)

## Impact

- Code: `internal/skilltag/dictionaries.go` (new aliases) + `internal/skilltag/*_test.go`.
- Deploy: shared deferred dict-only tail (`cmd/backfill-derive` + one `reindex`).
- Out of scope: soft skills, domain terms, category-from-description, work_mode/seniority (already done).
