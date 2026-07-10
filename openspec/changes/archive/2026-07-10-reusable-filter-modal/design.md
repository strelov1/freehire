## Context

`FilterModal.svelte` is hardwired to job search: it `new`s a `StagedFilters`,
hardcodes the job `RAIL` (`filterSections.ts`), and resolves facet defs from the job
`FACETS` registry. `SavedSearches.svelte` ("My filters") is embedded in
`FilterSummary.svelte`, which renders only in the desktop sidebar (`hidden md:block`)
— so on mobile, where only the modal opens (via `FilterEdgeTab`), saved searches are
unreachable. `SavedSearches` also carries board share/unshare/copy-link controls.
`/companies` ships its own always-open `CompanyFiltersPanel` + a bespoke mobile
drawer over `CompanyFilterStore` / `COMPANY_FACETS`. `/my/profile` renders the filter
sidebar/edge-tab/modal on every tab.

Key facts that shape the design:

- `FilterStore`, `StagedFilters`, and `CompanyFilterStore` all implement the shared
  `FacetStore` interface, and `FacetSection` already renders any `FacetStore`.
- The modal's `kind === 'facet'` pane renders `FacetSection`; company facets are all
  plain `facet`-kind, so company panes need **no new pane code**.
- Companies apply immediately (`setNow`); the modal is deferred (staged copy, commit
  on footer). Companies therefore need a deferred `StagedCompanyFilters`.
- `entryCount` currently reads `staged.value.facets[param].values` directly; the
  company value shape differs (`Record<string,string[]>`), so rail counts must be read
  through the uniform `staged.facet(param)` method.

## Goals / Non-Goals

**Goals:**

- One reusable modal shell + summary shell, reused by both the job and company
  catalogs, keeping the deferred-apply (staged) contract identical.
- "My filters" reachable wherever the modal opens (incl. mobile) as a deferred rail
  tab operating on the staged copy.
- Remove board share/unshare/copy-link from the in-context "My filters" control.
- `/companies` on the jobs pattern (summary sidebar + modal); `/my/profile` filters
  only on the Market coverage tab.

**Non-Goals:**

- No backend/API/DB or filter-serialization changes.
- No change to `/my/searches` or the board endpoints.
- No new facets or filter semantics; no change to job-modal panes' behavior.

## Decisions

### Shell + thin domain wrappers (over one parameterized file)

- **`FilterModalShell.svelte`** — reusable chrome only: backdrop, header, left rail
  (from a `rail: RailEntry[]` prop grouped by `sections`), footer (Clear all / Apply /
  preview "Show N"), error handling, seed-on-open. Depends on a minimal **staging
  contract** — `active: number`, `seed(): void`, `params(): URLSearchParams`,
  `commit(): void`, `clear(): void` — plus `entryCount(entry): number` and a
  `pane: Snippet<[RailEntry]>` for the active pane body. The seed source and commit
  target are closed over by the wrapper, so the shell stays domain-agnostic.
- **`FilterModal.svelte`** (job wrapper) — creates `StagedFilters`, passes job `RAIL`
  + job `entryCount` + the current if/else as the `pane` snippet. Keeps its public
  props (`store/seed/counts/exclude/railKeys/plain/extra/applyLabel/onApply/canApply/
  previewCount`) so `JobsView` and the profile change minimally. Adds the "My filters"
  rail entry (first, `SAVED` section) unless `railKeys` restricts the rail or no
  saved-search context applies.
- **`CompanyFilterModal.svelte`** (new) — creates `StagedCompanyFilters`, builds a
  rail from `COMPANY_FACETS` (single section, all `facet`-kind), `pane` renders
  `<FacetSection def store={staged} />`, `entryCount = staged.facet(param).values.length`.

Rationale: the job vs company `value` shapes diverge, so a single generic file would
either lose types or need casts; separate wrappers keep each typed to its own staged
store and pane set. Matches the "small, well-bounded units" guidance.

### Staging for companies

`StagedCompanyFilters` (`lib/stagedCompanyFilters.svelte.ts`) — a deferred copy of
`CompanyFilters` implementing `FacetStore` + `seed/params/commit/clear/active/value`,
mirroring `StagedFilters`. Pure logic → unit-tested with vitest (seed → mutate →
params/commit round-trips, `active`).

### My filters operates on the staged copy

Inside the modal the "My filters" tab drives the **staged** store, not the live one:
selecting a set seeds staged; "Save as new" persists staged; the footer applies. This
keeps the whole modal deferred and consistent. Requires `StagedFilters.apply(query)`
and a canonical-current getter off `staged.params()`. `SavedSearches.svelte`'s only
remaining mount is this tab (the sidebar no longer mounts it), so its `store` prop
becomes the staged store; `SavedSearchesView` on `/my/searches` is untouched.

### Summary shell

**`FilterSummaryShell.svelte`** (new) — shared summary chrome: heading + Reset all,
the All-filters button (active badge), empty state, chip-group rendering; takes
`groups: Group[]`, `active`, `onReset`, `onOpen`. `FilterSummary.svelte` (job) drops
`<SavedSearches>` and renders the shell; **`CompanyFilterSummary.svelte`** (new)
computes flat per-facet chip groups from `COMPANY_FACETS` and renders the shell.

### Consumers

- `CompaniesView` — desktop `CompanyFilterSummary` (chips + All-filters) + `FilterEdgeTab`
  (mobile) opening `CompanyFilterModal`. Delete `CompanyFiltersPanel.svelte` and the
  bespoke drawer.
- `my/profile` — render the filters sidebar + `FilterEdgeTab` + `FilterModal` only when
  `tab === 'coverage'`; the wrapper is used without a saved-search context (no My
  filters tab). CV readiness scores against the profile's default role.

## Risks / Trade-offs

- **Board sharing removed from the panel is a visible UI change.** Mitigated: the full
  share/unshare/copy management stays on `/my/searches`; the spec delta records the
  migration.
- **Staged "My filters" changes selection semantics** (select no longer commits
  immediately; it stages and applies on Show results). This is intentional for
  consistency but differs from today's behavior; captured in the saved-searches spec
  delta.
- **Refactor breadth** touches several components at once. Mitigated by TDD on the
  pure `StagedCompanyFilters` and `svelte-check` + visual verification for the wired
  UI; behavior of the job panes is preserved (the pane snippet is the existing if/else
  moved verbatim).
- **Two summaries could drift.** Mitigated by the shared `FilterSummaryShell` owning
  the chrome; only the group computation differs per catalog.
