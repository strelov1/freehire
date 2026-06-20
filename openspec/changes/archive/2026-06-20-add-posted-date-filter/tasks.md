## 1. Effective-date helper (single source of truth)

- [x] 1.1 Extract an exported effective-posting-date helper in `internal/jobview`
  (resolving the null/future `posted_at` → `created_at` fallback once) and route
  the existing display `PostedAt` through it, with a unit test covering present /
  null / future `posted_at`.

## 2. Search document carries `posted_ts`

- [x] 2.1 Add a derived `posted_ts` (unix seconds of the effective posting date)
  to `JobDocument` in `internal/search/document.go`, computed via the helper from
  1.1; index-only (not in the public job wire shape). Unit test: `posted_ts`
  equals the epoch of the effective date, including the null/future fallback.

## 3. Index declares `posted_ts` filterable

- [x] 3.1 Add `"posted_ts"` to `FilterableAttributes` in
  `internal/search/client.go` (leave `SortableAttributes` unchanged).

## 4. Filter builder parses `posted_within_days`

- [x] 4.1 Extend the search filter builder (`internal/search/query_filter.go`) so
  a positive integer `posted_within_days=N` adds `Gte("posted_ts", now - N*86400)`,
  with `now` injected (no global `time.Now`). Unit test: valid `N` → correct
  `Gte` fragment against an injected reference time; absent / empty / zero /
  negative / non-numeric → no date filter; composes with other facet filters.

## 5. Frontend state + URL round-trip

- [x] 5.1 Add `postedWithinDays: number | null` to `JobFilters`, round-trip it in
  `filtersToParams` / `filtersFromParams` as `posted_within_days`, and add a
  `setPostedWithinDays(n)` store method. (Uses `setSoon`, not `setNow`: the
  control is a dragged range input, so it debounces the reload exactly like the
  salary slider while writing the URL immediately.) Verify the round-trip
  (params ↔ state) for set and cleared values.

## 6. Frontend freshness slider

- [x] 6.1 Add the preset list `[Today=1, 3d, week=7, 2 weeks=14, month=30,
  3 months=90, Any=null]` alongside `SALARY_MAX` (which lives **in
  `FiltersPanel.svelte`**, not `facets.ts`); the list is component-local, mirroring
  the salary constants, since only the panel consumes it.
- [x] 6.2 Render a discrete-preset range slider at the **top** of
  `FiltersPanel.svelte` (rightmost = Any clears the param), wired to
  `setPostedWithinDays`, following the salary-slider pattern. Verify via
  `svelte-check` (+ `aria-valuetext` for the human label).

## 7. Verify end-to-end and operations

- [x] 7.1 Run `go build ./... && go vet ./... && go test ./...` and the web
  checks; confirm the new param filters search results. (Added integration test
  `TestSearchFiltersByPostedWithinDays` — real Meilisearch applies the posted_ts
  range filter and returns only the recent posting; svelte-check clean.)
- [x] 7.2 Record the post-deploy `cmd/reindex` requirement (so `posted_ts`
  populates existing jobs) — captured in design.md → Migration Plan and the
  proposal's Impact section.
