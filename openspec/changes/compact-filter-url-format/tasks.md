## 1. Backend: unify facet value parsing

- [ ] 1.1 Add a `splitFacetValues` helper in `internal/search/query_filter.go` that splits each raw value on `,`, flattens, and drops empty entries
- [ ] 1.2 Apply `splitFacetValues` to every `StringFacets` include-param read in `filterFromValues`
- [ ] 1.3 Apply `splitFacetValues` to every `StringFacets` `_exclude`-param read in `filterFromValues`
- [ ] 1.4 Unit tests covering: comma-separated single entry, repeated-key entries, mixed comma+repeated, empty/stray-comma values, and the exclude variant of each

## 2. Frontend: compact serialization and dual-format parsing

- [ ] 2.1 Add a shared split-and-flatten helper (or inline the same logic) used by both include and exclude parsing in `web/src/lib/facetModel.ts`
- [ ] 2.2 Update `filtersToParams` to write one comma-joined `p.set(def.param, values.join(','))` per facet with selections, instead of repeated `p.append`
- [ ] 2.3 Update `filtersFromParams` to decode both comma-joined and repeated-key entries via the shared helper
- [ ] 2.4 Tests/verification: round-trip a filter set through `filtersToParams` → `filtersFromParams` and confirm the same selections come back; confirm an existing repeated-key URL still parses correctly

## 3. Docs

- [ ] 3.1 Update the filter-format example and wording in `web/src/lib/docs/filters.ts` to show `skills=go,react` as primary, with a note that `skills=go&skills=react` still works
- [ ] 3.2 Regenerate `docs/API.md` via `gen:api-docs` and confirm the new wording appears

## 4. Verification

- [ ] 4.1 `go build ./...` and `go vet ./...`
- [ ] 4.2 `go test ./...`
- [ ] 4.3 Manually load a job-search page, select several skills, confirm the URL uses the compact form, then reload the page from that URL and confirm the same filters are applied
- [ ] 4.4 Manually load an old-style URL with repeated `skills=` keys and confirm filters still apply correctly
