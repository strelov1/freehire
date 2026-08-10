## Why

Job-search filter URLs grow one query pair per selected value (`skills=go&skills=react&skills=aws&...`), so a filter set with dozens of skills produces URLs thousands of characters long — unwieldy to share, paste, or read. A comma-separated encoding (`skills=go,react,aws`) carries the same information far more compactly, and can be added without breaking any URL that already exists.

## What Changes

- The job-filter query encoding accepts comma-separated values within a single query-string entry (`skills=go,react`) for every list-valued facet param (the `StringFacets` set: skills, skills_exclude, regions, category, and the rest) — the `<param>_exclude` and `<param>_mode=and` modifiers are unaffected.
- The existing repeated-key encoding (`skills=go&skills=react`) keeps working — both forms are parsed the same way, so old bookmarks, saved searches, subscription alert URLs, and third-party clients (`freehire-cli`) that build repeated-key URLs need no migration.
- The web app's filter UI now serializes selections into the URL using the compact comma-separated form instead of repeated keys.
- The public API docs (`/docs/api`, generated `docs/API.md`) show the compact form as the primary example and note the repeated-key form still works.

## Capabilities

### New Capabilities
- `filter-query-encoding`: how list-valued job-filter params are encoded in a query string and decoded on read — accepts both comma-separated and repeated-key forms, on both the web app's own URL state and the public `/jobs*` API.

### Modified Capabilities
- `api-documentation`: the filter vocabulary documentation (`Filter vocabulary is documented in depth` requirement in `api-documentation`) now shows the compact comma-separated form as the documented convention alongside the existing repeated-key form.

## Impact

- `internal/search/query_filter.go` — `filterFromValues` gains a comma-split-and-flatten step applied uniformly to every facet's include and exclude values.
- `web/src/lib/facetModel.ts` — `filtersToParams` serializes each facet's selected values into one comma-joined query entry; `filtersFromParams` decodes both comma-joined and repeated-key entries.
- `web/src/lib/docs/filters.ts` and the generated `docs/API.md` — filter-format wording and examples updated.
- Not touched: company filters (`stagedCompanyFilters.ts`/`companyFilters.ts`, a separate codec) and `../freehire-cli` (already works unchanged against the backward-compatible backend).
