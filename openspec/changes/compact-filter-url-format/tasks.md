## 1. Backend: unify facet value parsing

- [x] 1.1 Add a `splitFacetValues` helper in `internal/search/query_filter.go` that splits each raw value on `,`, flattens, and drops empty entries
- [x] 1.2 Apply `splitFacetValues` to every `StringFacets` include-param read in `filterFromValues`
- [x] 1.3 Apply `splitFacetValues` to every `StringFacets` `_exclude`-param read in `filterFromValues`
- [x] 1.4 Unit tests covering: comma-separated single entry, repeated-key entries, mixed comma+repeated, empty/stray-comma values, and the exclude variant of each

## 2. Frontend: compact serialization and dual-format parsing

- [x] 2.1 Add a shared split-and-flatten helper (or inline the same logic) used by both include and exclude parsing in `web/src/lib/facetModel.ts`
- [x] 2.2 Update `filtersToParams` to write one comma-joined `p.set(def.param, values.join(','))` per facet with selections, instead of repeated `p.append`
- [x] 2.3 Update `filtersFromParams` to decode both comma-joined and repeated-key entries via the shared helper
- [x] 2.4 Tests/verification: round-trip a filter set through `filtersToParams` → `filtersFromParams` and confirm the same selections come back; confirm an existing repeated-key URL still parses correctly
- [x] 2.5 (found in review) `URLSearchParams.toString()` percent-encodes commas to `%2C`; added `urlSearchString.ts#toSearchString` and wired it into every place that writes the address bar from a facet query: `UrlSyncedState#write`, `JobsView#openSwipe`, `SwipeDeck#close`, `SavedSearchesView`'s "Open" link

## 3. Docs

- [x] 3.1 Update the filter-format example and wording in `web/src/lib/docs/filters.ts` to show `skills=go,react` as primary, with a note that `skills=go&skills=react` still works
- [x] 3.2 Regenerate `docs/API.md` via `gen:api-docs` and confirm the new wording appears

## 4. Verification

- [x] 4.1 `go build ./...` and `go vet ./...` — clean; also ran `go vet -tags=integration ./...` per AGENTS.md pre-push convention, clean
- [x] 4.2 `go test ./...` — all packages pass, no failures
- [x] 4.3 / 4.4 Skipped live browser verification: the dev Docker stack is occupied by other concurrent worktrees on this machine (port 5432 held, no Redis instance available for the recently-added rate-limit dependency), and standing up an isolated stack was judged not worth the port/resource risk for this change — decided with the user. Covered instead by: `facetModel.test.ts`'s explicit round-trip test (compact serialize → parse) and backward-compat test (old repeated-key URL → same filter state as the new form), plus `urlSearchString.test.ts` verifying the literal-comma address-bar output and that it re-parses correctly — the same mechanics a live browser check would exercise.
