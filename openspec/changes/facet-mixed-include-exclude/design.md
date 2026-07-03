## Context

Job-search filters live in the web client as a per-facet `FacetState { values[],
exclude, matchAll }` (`web/src/lib/filters.ts`). The single `exclude` boolean makes a
facet all-include or all-exclude, so "include nodejs and exclude php/.net" is
unrepresentable in one facet. The backend already accepts `?skills=nodejs` and
`?skills_exclude=php` in the same request (`internal/search/query_filter.go` ANDs the
excludes with the include group), so this is purely a web-client limitation.

The same `FacetState` and control components (`FacetSection`, `PillGroup`,
`SearchSelect`, `RemoteSearchSelect`) drive both the sidebar and the deferred-edit
`FilterModal` (via `StagedFilters`), and the company filter store satisfies the same
`FacetStore` interface. The web project has no test runner today (verify is
`svelte-check` + visual). A prototype of the target interaction was built and approved.

## Goals / Non-Goals

**Goals:**
- Let one facet hold included and excluded values simultaneously.
- Keep the URL/API contract unchanged (`?param` / `?param_exclude` / `?param_mode=and`)
  so shared links and saved searches keep working.
- One consistent mental model across control types (a value is off / included /
  excluded everywhere).
- Unit-test the pure filter logic (model transitions + URL round-trip).

**Non-Goals:**
- No backend, Meilisearch, or search-API change; `query_filter.go` and its tests stay
  untouched.
- No component-level test harness (Svelte controls stay visually verified).
- No change to which facets are excludable, nor to the company filter's include-only
  behavior.

## Decisions

**1. Two arrays over a per-value tag.** `FacetState` becomes `{ include: string[],
exclude: string[], matchAll: boolean }`. This maps 1:1 to the existing URL params and
to the backend's group structure, and keeps `matchAll` naturally scoped to the include
set. Alternative — a single `values: {value, sign}[]` — was rejected: it complicates
serialization and every chip render keys on a plain string today.

**2. A `setSign` primitive with semantic wrappers in the store.** The store exposes
`setSign(param, value, 'off'|'include'|'exclude')` plus intent-named wrappers so
components stay dumb and the transitions stay unit-testable:
- `cycle` (pills): off → include → exclude → off
- `pick` (select dropdown): off → include, else → off
- `toggleSign` (select chip control): include ↔ exclude
- `add` (token input): → include; `remove`: → off
`setExclude` (whole-facet) is removed. `FacetStore` (the shared interface) and
`StagedFilters` mirror this surface; the company store implements them as include-only
(cycle/toggleSign never fire on non-excludable facets).

**3. Control rendering keys off each value's sign, not a facet-wide flag.**
`PillGroup` renders three states; `SearchSelect`/`RemoteSearchSelect` render an
include chip group and a destructive exclude group with a per-chip include↔exclude
toggle. `FacetSection`/`FacetHeader` drop the whole-facet Exclude toggle but keep the
Clear and the match-all (Any/All) toggle (still gated on `include.length > 1`).
`FilterSummary` renders included and excluded values distinctly.

**4. Backward-compatible parse.** `filtersFromParams` reads `param` → include,
`param_exclude` → exclude. If a value appears in both (malformed/legacy URL), exclude
wins and it is dropped from include, preserving the "one sign per value" invariant.

**5. Add vitest for the pure logic only.** Vite is already the web toolchain, so a
`vitest` dev-dependency + `test` script is a minimal addition. Tests cover
`setSign`/`cycle`/`pick`/`toggleSign`, `filtersToParams`, `filtersFromParams`, the
round-trip (`canonicalQuery`), and `activeFilterCount`. Components remain
visually verified (headless-Chrome on the live skills facet).

## Risks / Trade-offs

- **Three-state pills are less discoverable** (second click = exclude) → mitigate with
  the destructive style + a `title` tooltip naming the next state; the prototype
  confirmed the interaction reads clearly.
- **Broad mechanical churn across ~11 files** touching a shared type → mitigate by
  landing the model + store + tests first (RED/GREEN on pure logic), then updating
  controls under `svelte-check`, which flags every `FacetState` shape mismatch.
- **Company filter store shares the interface** → adding no-op-ish include-only
  wrappers keeps it compiling; excludable is already false there, so behavior is
  unchanged.

## Migration Plan

Pure client change; ships with the normal web deploy. No data migration. Old shared
URLs and saved searches parse unchanged. Rollback is reverting the web bundle — the
URL contract is identical, so a rolled-back client still reads today's links.

## Open Questions

None — model, store API, and both control interactions were settled in brainstorming
and validated with the approved prototype.
