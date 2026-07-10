## Why

The two-pane "All filters" modal is hardwired to job search (it constructs a job
`StagedFilters`, hardcodes the job rail, and looks facets up in the job registry),
so it cannot be reused for the companies catalog, and "My filters" (saved searches)
lives only in the desktop sidebar — invisible on mobile, where only the modal opens.
The filters panel also carries board share/unshare controls that belong on the
dedicated `/my/searches` page, and the `/my/profile` page shows the filters on every
tab though they only apply to Market coverage.

## What Changes

- Refactor the modal into a reusable **shell** (`FilterModalShell`: backdrop,
  header, rail, footer, deferred apply/preview) plus thin domain wrappers — the job
  `FilterModal` and a new `CompanyFilterModal` — so both catalogs share one chrome.
- Extract a **`FilterSummaryShell`** for the sidebar (heading, Reset all, All-filters
  button, chip groups, empty state); the job `FilterSummary` and a new
  `CompanyFilterSummary` render over it.
- Add **"My filters" as a rail tab** inside the modal (first entry), operating on the
  staged copy — so saved searches are reachable on mobile and stay consistent with
  the deferred model.
- **Remove** the board share/unshare/copy-link affordance from the "My filters"
  panel (`SavedSearches.svelte`). **BREAKING (UI)**: board management now happens only
  on `/my/searches`.
- Move `/companies` to the jobs pattern: a summary sidebar + the reused two-pane
  modal (desktop button + mobile edge tab), replacing the always-present
  `CompanyFiltersPanel` and its bespoke drawer.
- On `/my/profile`, render the filters sidebar / edge tab / modal **only on the
  Market coverage tab**.

## Capabilities

### New Capabilities

_None — this change refactors and relocates existing filter UI; no new user-facing
capability._

### Modified Capabilities

- `filter-modal`: the modal + summary sidebar are generalized so a non-job catalog
  reuses them; "My filters" becomes a deferred rail tab; the summary sidebar no
  longer carries the saved-search controls.
- `saved-searches`: the "My filters" control relocates into the modal as a deferred
  tab (staged, applied on Show results); the board share/unshare affordance in the
  filters panel is removed (it remains on `/my/searches`).
- `web-frontend`: the companies list switches from an always-present filter-control
  sidebar to the jobs-style summary sidebar + two-pane "All filters" modal; the
  `/my/profile` page shows its filters only on the Market coverage tab.

## Impact

- Web only. Affected: `web/src/lib/components/filters/*` (new `FilterModalShell`,
  `FilterSummaryShell`; refactored `FilterModal`, `FilterSummary`; new
  `CompanyFilterModal`, `CompanyFilterSummary`), `SavedSearches.svelte`, new
  `lib/stagedCompanyFilters.svelte.ts`, `CompaniesView.svelte` (delete
  `CompanyFiltersPanel.svelte`), and `routes/my/profile/+page.svelte`.
- No backend, API, DB, or filter-serialization changes. `/my/searches` and the board
  endpoints are untouched.
