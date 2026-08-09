## Why

`internal/roletag` classifies a job's role from its title alone, but the
[AI Engineering Field Guide](https://github.com/i-strelov/ai-engineering-field-guide)'s
job-market analysis shows title is an unreliable signal for AI roles: the
single title "AI Engineer" spans three functionally different profiles
(AI-First 69.4%, AI-Support 28.5%, classical ML rebranded 1.8%), and a
skill-signature k-means clustering surfaces six stable archetypes — the
largest, "RAG app builder" (25.6% of the market), has no dedicated facet today.
Separately, `internal/skilltag`'s dictionary has a confirmed gap on `MCP`
(159 free-text mentions in prod AI-category jobs, 1 tagged) and an
under-tagging gap on `RAG` itself (15.2% tagged vs. the field guide's 35.9%
baseline, because the bare acronym is gated to résumés only to avoid a
collision with "RAG status"). `forward_deployed_engineer` already resolves
1,570 prod jobs from one exact title phrase, but misses the bare "FDE" title
and the field-guide-documented synonym titles (Applied AI Engineer,
Deployment Engineer).

## What Changes

- Add a new derived, dict-only facet `ai_archetype` (`internal/aiarchetype`,
  new package) that classifies a job into one of six skill-signature
  archetypes — `rag_app_builder`, `agent_builder`, `cloud_ml_platform_engineer`,
  `devops_infra_engineer`, `ml_trainer_researcher`, `fullstack_ai_engineer` —
  from its already-resolved `skills` and `category`. Scoped to
  `category ∈ {ai_engineering, ml_ai}`. Wired into `internal/search` as a new
  filterable Meilisearch facet, following the same "compute at index time,
  never persisted" pattern as `roletag`'s `roles` facet.
- Widen `forward_deployed_engineer`'s title aliases in `internal/roletag` to
  match the field guide's own matching rule (bare `FDE`, `forward deploy*`),
  and add two new named roles, `applied_ai_engineer` and `deployment_engineer`,
  for the synonym titles the field guide documents as the same work under a
  different name — kept as separate slugs, not merged into FDE, to preserve
  title fidelity.
- Add `mcp` to `internal/skilltag`'s word-alias dictionary.
- Move the `RAG` acronym from résumé-only (`resumeAcronyms`) to a
  category-scoped strong match on job postings
  (`category ∈ {ai_engineering, ml_ai}`), closing the under-tagging gap
  without reopening the "RAG status" collision on the rest of the catalogue.

## Capabilities

### New Capabilities
- `ai-role-archetype`: derives a dict-only `ai_archetype` facet from a job's
  skills and category, for the six AI-engineering skill-signature archetypes;
  exposed as a filterable Meilisearch facet.

### Modified Capabilities
- `role-facet`: `internal/roletag`'s named-role table gains broader
  `forward_deployed_engineer` aliases and two new named roles
  (`applied_ai_engineer`, `deployment_engineer`).
- `skill-tag-matching`: the dictionary gains an `mcp` word alias and a
  category-scoped strong match for the `RAG` acronym on job postings.

## Impact

- New package `internal/aiarchetype` (pure function, no persistence, no
  migration).
- `internal/search/document.go` (`JobDocument`, `FromJob`), `client.go`
  (`FilterableAttributes`), `query_filter.go` (`StringFacets`) — new facet
  wiring, same pattern as `roles`.
- `internal/roletag/roletag.go` — `namedRoleTable` additions/edits only.
- `internal/skilltag/dictionaries.go` — `wordAliases`, `sharedAcronyms`/
  `resumeAcronyms` additions; `Parse` gains a category-aware acronym path.
- No schema migration. No backfill: `ai_archetype` is computed at Meilisearch
  index time from already-persisted `jobs.skills`/`jobs.category`, so a full
  `cmd/reindex` (already required whenever a new filterable attribute is
  added, per `internal/search/AGENTS.md`) picks up historic jobs.
