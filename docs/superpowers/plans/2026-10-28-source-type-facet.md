# Source-type facet Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A `source_type` search facet (`ats` / `company` / `aggregator` / `social`) with `_exclude`, reachable from the API and from the filter modal's Source pane.

**Architecture:** `sources.ProviderKind` stays the one classifier and gains `social`. The value is written onto each Meilisearch document at index time; `search.FromJob` takes it as a parameter that its three callers resolve from `sources.Taxonomy()`. The facet then plugs into the existing `StringFacets` machinery (filter, `_exclude`, `/jobs/facets`, `ignored_values`).

**Tech Stack:** Go, Meilisearch, SvelteKit.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-10-28-source-type-facet-design.md`.
- Layering: `search` (6) may not import `ingest` (7); the vocabulary lives in `internal/dict/vocab`.
- Rollout: live Meili settings patch BEFORE the binary, full reindex after.
- English only in code, comments, commits.

---

### Task 1: Classifier and vocabulary

**Files:**
- Modify: `internal/dict/vocab/vocab.go` — add `SourceTypeValues = []string{"ats", "company", "aggregator", "social"}`.
- Modify: `internal/ingest/sources/source.go` — add `type social interface{ social() }` marker.
- Modify: `internal/ingest/sources/hackernews.go` — `func (hackernews) social() {}`.
- Modify: `internal/ingest/sources/classify.go` — `KindSocial`; social marker and the `telegram` name win first; `employer` name → `KindCompany`; `SourceTypes(reg)` returning `func(source string) string` that maps `KindOther` to `""`.
- Modify: `internal/ingest/pipeline/pipeline.go` `aggregatorGate` — test the `aggregator` marker via `AggregatorProviders` membership instead of `ProviderKind == KindAggregator`.
- Test: `internal/ingest/sources/classify_test.go`.

Tests: hackernews → social, telegram → social, employer → company, greenhouse → ats, adzuna → aggregator, manual → other; every `ProviderKind` over `Taxonomy()` is in `vocab.SourceTypeValues`.

### Task 2: Index document and filter

**Files:**
- Modify: `internal/search/search/document.go` — `SourceType string \`json:"source_type,omitempty"\``; `FromJob(j, desc, sourceType string)`.
- Modify: `internal/search/search/query_filter.go` — `"source_type": "source_type"` in `StringFacets`.
- Modify: `internal/search/search/query_values.go` — `"source_type": oneOf(vocab.SourceTypeValues)`.
- Modify: `internal/search/search/client.go` — `"source_type"` in `FilterableAttributes`.
- Modify callers: `cmd/reindex/main.go`, `cmd/search-drain/indexer.go`, `internal/ingest/linkimport/linkimport.go` — resolve `sources.SourceTypes(sources.Taxonomy())` once, pass `typeOf(j.Source)`.
- Test: existing `document_test.go`/`query_filter_test.go` patterns.

### Task 3: Web

**Files:**
- `cmd/gen-contracts/main.go` — emit `SourceType` / `SOURCE_TYPE_VALUES`; `make gen-contracts`.
- `web/src/lib/labels.ts` — `SOURCE_TYPE_LABELS`: ATS, Company site, Aggregator, Social.
- `web/src/lib/facets.ts` — `{ param: 'source_type', label: 'Source type', control: 'pills', options: SOURCE_TYPE, excludable: true }` before `source`.
- `web/src/lib/filterSections.ts` — the `source` rail entry becomes kind `'source'`.
- `web/src/lib/components/filters/FilterModal.svelte` — the `source` pane renders the `source_type` chips then the `source` select; `entryCount` sums both.
- `web/src/lib/types.ts`, `StatusBoard.svelte` — `ProviderKind` gains `'social'`.

### Task 4: API contract and docs

- `web/static/openapi.yaml` — `source_type` and `source_type_exclude` params next to `role_type`.
- `internal/search/search/AGENTS.md` — note the settings-before-binary order for `source_type`.

### Task 5: Verify and ship

- `gofmt -l .`, `go vet ./...`, `go vet -tags=integration ./...`, `go test ./...`, web `pnpm check` + tests.
- PR, CodeRabbit, merge; patch live Meili settings before the release flips; reindex.
