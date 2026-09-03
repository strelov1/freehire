## Why

A job carries two dates and the feed can only filter on one of them. `posted_at` is
what the board claims; `created_at` is when we first saw the posting. Some boards
rewrite the first on every crawl, so `?posted_within_days=3` currently returns
postings whose own card reads `Open 72d` beside `37 minutes ago` — the filter and the
badge disagree in public, and the reader has no way to ask the question they actually
meant: *don't show me anything that has been sitting here for months*.

The system already knows the board is lying — `jobreality` computes `fake_freshness`
from exactly this gap — but that knowledge only reaches a badge, never a filter.

## What Changes

- Add an `open_within_days=N` filter to `GET /api/v1/jobs/search`, bounding on the
  posting's first-seen date (`created_at`), which no source can rewrite. It composes
  with the existing filters (AND) and follows the same "invalid value imposes no
  restriction" rule as `posted_within_days`.
- Add `created_ts` (unix seconds) to the search document and to the index's
  filterable attributes — the numeric field the range filter needs, mirroring the
  existing `posted_ts`. Document-only: never served to clients.
- The filter modal's `Posted` pane carries **two** bounds: `Open within` (ours) above
  `Posted within` (the source's), each labelled for whose date it is.
- The `Posted` select above the list becomes `Open`, writing `open_within_days`.
  `posted_within_days` keeps its meaning and its URL spelling, so shared links and
  saved searches are unaffected.
- **Remove** the `Hide evergreen` toggle above the list. The whole `reality` facet
  moves into the `Posted` pane, beneath the age bounds, and its standalone
  `Posting reality` rail entry retires — one pane now answers "how current is this
  posting", instead of splitting the question across two tabs and a button.
- The new control ships dark behind `PUBLIC_OPEN_WITHIN` (default OFF), because the
  filter returns a near-empty feed until a full index rebuild has written `created_ts`
  into every document — a thin feed that nothing alerts on.

No breaking changes: every existing parameter keeps its spelling and its meaning.

## Capabilities

### New Capabilities

None. The change extends three existing capabilities rather than introducing a
concept of its own — the second date already exists and is already served as
`created_at`; only the ability to filter on it is new.

### Modified Capabilities

- `job-search`: a second freshness bound, `open_within_days`, filtering on first-seen
  date rather than the source's stated posting date.
- `filter-modal`: the `Posted` pane gains the `Open within` bound and hosts the
  `reality` facet; the standalone `Posting reality` rail entry is removed.
- `jobs-list-controls`: the above-list select bounds `open_within_days` rather than
  `posted_within_days`; the `Hide evergreen` toggle is removed.

## Impact

**Go**
- `internal/search/search/document.go` — `JobDocument.CreatedTS`
- `internal/search/search/client.go` — `created_ts` in `FilterableAttributes`
- `internal/search/search/query_filter.go` — the `open_within_days` bound
- `internal/search/search/query_params.go` — `open_within_days` in `scalarFilters`,
  or the endpoint reports a working filter as an unknown param

The three indexers (`cmd/reindex`, `cmd/search-drain`, `internal/ingest/linkimport`)
need no edit: all of them build documents through the single `search.FromJob`, which
is where `posted_ts` is already derived and where `created_ts` joins it.

**Web**
- `facetModel.ts` (`openWithinDays`, serialize/parse/count), `filters.ts`,
  `stagedFilters.svelte.ts`, `features.ts` (`openWithinEnabled`)
- `FilterModal.svelte`, `JobsView.svelte`, `ListToolbar.svelte`, `FilterSummary.svelte`,
  `filterSections.ts`

**Contract**
- `web/static/openapi.yaml` and `web/src/lib/docs/api-spec.ts`

**Deploy** — ordering is load-bearing and documented in
`internal/search/search/AGENTS.md` ("Adding a filterable attribute"): a binary that
asks for an attribute the live index has not declared turns every filtered search into
a 500. Settings patch first, binary second, full rebuild third, flag last.

**Not in scope**
- `internal/search/searchintent` keeps mapping "posted this week" to
  `posted_within_days`. That phrase is literally about the posting date; teaching the
  LLM the second bound is its own change with its own prompt evaluation.
- The in-flight `freshness.ts` badge work in the main checkout is untouched. Its
  `daysSince` helper overlaps nothing here — this change adds no client-side date
  arithmetic — but the two should be read together once that branch lands.
