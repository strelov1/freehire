## 1. Non-tech title detector

- [x] 1.1 Add `nonTechTable` (confident non-tech role nouns, EN, whole-word) to `internal/classify/dictionaries.go`
- [x] 1.2 Add `classify.IsNonTech(title string) bool` with unit tests: confident non-tech → true, tech titles → false, substring/word-boundary and shared-term ("engineer") negatives

## 2. Tech-category partition

- [x] 2.1 Add `enrich.TechCategories` (CategoryValues minus NonTechCategories minus `other`) as the single source of truth for "recognized technical category"
- [x] 2.2 Test that TechCategories / NonTechCategories / {`other`} partition `CategoryValues` (no overlap, full cover)

## 3. is_tech derivation

- [x] 3.1 Add `IsTech *bool` to the `jobderive.Derived` result
- [x] 3.2 Derive it in `jobderive.Derive`: tech category → true; category ∈ NonTechCategories OR `IsNonTech(title)` → false; else nil. Unit tests for all four states, tech-wins precedence

## 4. Persistence (DB + aggregate)

- [x] 4.1 Migration: add nullable `jobs.is_tech boolean`
- [x] 4.2 Thread `IsTech` through the `internal/job` aggregate (field + `job.New` + `job.FromRow`)
- [x] 4.3 Update `UpsertJob` and the backfill-derive update query in `internal/db/queries/*.sql`; run `make sqlc`
- [x] 4.4 Verify `cmd/backfill-derive` writes `is_tech` (it re-derives via jobderive); DB integration/handler test as applicable

## 5. Served wire shape

- [x] 5.1 Add `is_tech` string-enum field to `jobview` (`"tech"`/`"non_tech"`, omitted when unknown), mapped from the aggregate `*bool`
- [x] 5.2 Unit test `jobview.FromDomain` for the three states

## 6. Search facet + filter

- [x] 6.1 Include `is_tech` in the search document (top-level string facet, like `roles`) and add it to `facetSettings` FilterableAttributes
- [x] 6.2 Wire `is_tech` into `internal/search` facet request + `query_filter` mapping; test filter (tech excludes non_tech + unknown) and facet distribution
- [x] 6.3 Add the Tech / Non-tech control to `web/src/lib/facets.ts` + labels (FilterModal), values `tech`/`non_tech`

## 7. Contracts + verification

- [x] 7.1 Regenerate TS contracts (`cmd/gen-contracts`) so the `is_tech` field reaches the frontend types
- [x] 7.2 `go build ./... && go vet ./... && go test ./...`; web `svelte-check` for touched files
- [x] 7.3 Confirm end-to-end locally: ingest a job → `is_tech` persisted → served in jobview → filterable in search (reindex first)
