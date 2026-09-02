## Why

A user wrote in asking for a way to sort by date, because the postings they hit
"aren't in any particular order" and read as stale. Every capability they asked
for already exists in the backend and is unreachable with a mouse:

- `sort=posted_at` is an accepted, sortable attribute — but the client's sort
  select is hidden behind `matchFilterAvailable && matchSortEnabled(env)`, so a
  signed-out visitor never sees it at all.
- The select lies when it is visible. `newest` is `DEFAULT_JOB_SORT`, and
  `filtersToParams` omits the default — so with query text the request carries no
  `sort`, the engine keeps relevance order, and the control still reads "Newest".
- `posted_within_days` has a control, buried as the last entry of the modal's
  third rail section, under the heading `REQUIREMENTS & ELIGIBILITY` — where a
  posting's age is not a requirement of the candidate.
- The `reality` facet (`fresh` / `stale` / `likely-evergreen`) is defined in
  `FACETS` and understood by the backend, but has **no rail entry at all**. It is
  reachable only by hand-editing the URL. Nothing caught this: the company rail
  has a completeness test, the jobs rail does not.

Two of these are the same defect the reporter described — the catalogue can
already answer "show me recent postings" and never offered to.

## What Changes

- The feed's sort vocabulary gains an explicit `relevance` value, and its default
  becomes contextual: `relevance` with query text, `newest` without. `newest`
  under a query now genuinely emits `sort=posted_at` instead of silently serving
  relevance. This makes the client match the behaviour `job-search` already
  specifies for the endpoint; no endpoint change.
- The sort select is shown whenever it holds more than one option, rather than
  only to a signed-in user with profile skills. `relevance` is offered only under
  query text; `match` keeps its existing profile-and-flag gate.
- A **Posted** select joins the sort control above the list, over the existing
  `FRESHNESS_PRESETS`. Same store as the modal slider, so the two stay in step.
- A **Hide evergreen** toggle joins them, writing
  `reality_exclude=likely-evergreen` — the use the facet's own comment names as
  the common one.
- `ListToolbar` renames `sortControl` to `controls` (it now hosts three) and stops
  suppressing the whole desktop row when the total is rendered elsewhere, which is
  what hides these controls on a company page.
- The modal rail moves `Posted` into `ROLE` after `Experience`, and gains a
  `Posting reality` entry.
- A completeness test asserts every `FACETS` param has a rail entry or is on a
  documented exception list, so the next facet cannot fall out silently.

Deferred to its own change, deliberately: filtering or sorting on `updated_at`.
The column became meaningful when `RefreshUnchangedJob` stopped stamping it on
every crawl, but it is absent from the Meilisearch document, so it needs a schema
field, settings change, full reindex, and a two-step release. See design.md.

## Capabilities

### New Capabilities

- `jobs-list-controls`: the control row above the jobs list — the sort select's
  vocabulary, its contextual default and visibility rule, the freshness select,
  and the evergreen toggle, plus how each is mirrored into the URL.

### Modified Capabilities

- `web-frontend`: the "Jobs browse sort control" requirement is removed. It
  describes a **Date posted** / **Recently added** pair that no longer exists in
  the code, and `jobs-list-controls` supersedes it.
- `filter-modal`: the rail's section membership changes (`Posted` moves to
  `ROLE`), the rail gains a `Posting reality` entry, and the rail acquires a
  completeness rule against `FACETS`.
- `header-filter-trigger`: the toolbar requirement's wording covers one sort
  control; it now hosts three list controls.

## Impact

Client only. No Go, no SQL, no Meilisearch settings, no reindex, no migration.

- `web/src/lib/facetModel.ts` — `JobSort`, the contextual default, serialization
- `web/src/lib/filters.ts` — a `setNow` freshness setter beside the slider's
- `web/src/lib/components/JobsView.svelte` — the three controls
- `web/src/lib/components/ListToolbar.svelte` — prop rename, desktop-row condition
- `web/src/lib/components/CompaniesView.svelte` — prop rename at the call site
- `web/src/lib/filterSections.ts` — rail membership
- `web/src/lib/facetModel.test.ts`, `web/src/lib/filterSections.test.ts` — tests

Shared links and saved searches carrying `sort=match` keep working unchanged.
