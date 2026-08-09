## 1. skilltag: MCP alias + category-scoped RAG acronym

- [x] 1.1 Add `"mcp": "mcp"` to `wordAliases` in `internal/skilltag/dictionaries.go`
- [x] 1.2 Add a `categoryScopedAcronyms` table (mirroring `sharedAcronyms`'s
      shape) with `RAG → rag`, gated to an allow-list of `ai_engineering`,
      `ml_ai`. Leave `RAG` in `resumeAcronyms` unchanged — résumé parsing
      stays acronym-scoped independent of category (design.md decision 5)
- [x] 1.3 Add a `WithAcronymCategory(category string)` functional option to
      `skilltag.Parse`; when set and the category is in a scoped acronym's
      allow-list, resolve that acronym on job text (default: unset, no
      behavior change for existing callers)
- [x] 1.4 Unit tests: MCP resolves as a plain word alias; RAG resolves for
      `ai_engineering`/`ml_ai` category via the new option; RAG does not
      resolve for other categories or when the option is unset; existing
      résumé-scoped RAG behavior is unchanged

## 2. roletag: FDE alias coverage + two new named roles

- [x] 2.1 Broaden `forward_deployed_engineer`'s aliases to also match bare
      `FDE` and the `forward deploy` prefix (covering the hyphenated
      `forward-deployed` spelling — check the existing hyphen-as-word-boundary
      handling other entries in `namedRoleTable` already document)
- [x] 2.2 Add `applied_ai_engineer` ("Applied AI Engineer") and
      `deployment_engineer` ("Deployment Engineer") to `namedRoleTable` as
      their own slugs
- [x] 2.3 Unit tests: bare "FDE" title resolves to `forward_deployed_engineer`;
      "Forward-Deployed Engineer" resolves; "Applied AI Engineer" resolves to
      `applied_ai_engineer` and NOT `forward_deployed_engineer`; "Deployment
      Engineer" resolves to `deployment_engineer`; catalog includes labels for
      both new slugs

## 3. New package internal/aiarchetype

- [x] 3.1 Create `internal/aiarchetype/aiarchetype.go` with
      `Derive(skills []string, category string) string` implementing the
      six-rule ordered priority table from design.md
- [x] 3.2 Unit tests per spec scenario: each archetype's rule matches in
      isolation; `rag_app_builder` wins over `agent_builder` on overlapping
      skills; out-of-scope category yields ""; no-match yields ""

## 4. Wire category into job ingest

- [x] 4.1 Update `internal/jobderive` to pass the job's resolved category into
      `skilltag.Parse` via `WithAcronymCategory`
- [x] 4.2 Integration test: a job description mentioning bare "RAG" in
      category `ai_engineering` ends up with `rag` in `jobs.skills` after
      `jobderive.Derive`

## 5. Wire ai_archetype into search

- [x] 5.1 Add `AIArchetype string` (or equivalent) field to `JobDocument` in
      `internal/search/document.go`; set it via `aiarchetype.Derive` in
      `FromJob`
- [x] 5.2 Add `"ai_archetype"` to `FilterableAttributes` in
      `internal/search/client.go` `facetSettings()`
- [x] 5.3 Add `"ai_archetype": "ai_archetype"` to `StringFacets` in
      `internal/search/query_filter.go`
- [x] 5.4 Add a filterability test sibling to `TestRolesIsFilterable` in
      `internal/search/settings_test.go`
- [x] 5.5 Integration/unit test: `FromJob` sets `ai_archetype` correctly for a
      job with `rag_app_builder`-matching skills and category, and leaves it
      empty for a non-AI category

## 6. Reindex

- [ ] 6.1 Push updated Meilisearch settings (filterable attributes) to prod
      ahead of the reindex, per `internal/search/AGENTS.md`'s ordering
      requirement for a new filterable attribute
- [ ] 6.2 Run a full `cmd/reindex` so `ai_archetype` (and the widened
      `roles`/`skills` from steps 1-2) reach every historic job — coordinate
      with the ingest-slot flock and the "never stack with reindex-companies"
      rule
