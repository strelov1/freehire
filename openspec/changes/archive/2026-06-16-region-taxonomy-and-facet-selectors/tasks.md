# Tasks

> Implemented ahead of this tracking change; all tasks reflect work that is done
> and verified (`go test ./...`, `svelte-check`, web production build all green).

## 1. Region vocabulary

- [x] 1.1 Rewrite `enrich.RegionValues` to the macro-only set `{global,
      north_america, latam, eu, uk, mena, africa, apac, cis}`; update the
      `Enrichment.Regions` field doc.
- [x] 1.2 Update `internal/location` `regionCountries` (US→`north_america`;
      RU + CIS + Central Asia merged into `cis`) and `nameToRegion` (drop
      `americas`/`emea`/`eea`; `central asia`→`cis`).
- [x] 1.3 Update `internal/location` parser tests for the new mappings.
- [x] 1.4 Update `enrich.TestValidateAcceptsRegions` fixtures to current vocab.
- [x] 1.5 Regenerate web TS contracts (`make gen-contracts`).
- [x] 1.6 Update the curated web `REGION` filter list in `facets.ts`.

## 2. Distribution-driven Skills / Countries selects

- [x] 2.1 Add `dynamic` to `FacetDef` and `count` to `FacetOption`; flip
      `skills`/`countries` to `control: 'select', dynamic: true`.
- [x] 2.2 Build dynamic options from the facet distribution in `FacetSection`
      (busiest-first, selected-but-absent values retained); show counts in
      `SearchSelect`; label countries via `Intl.DisplayNames`.
- [x] 2.3 Fetch the distribution in `JobsView` (debounced, monotonic
      stale-response guard) and thread `counts` through `FiltersPanel` →
      `FacetSection`; forward `counts` from `AnalyticsView` too.

## 3. Robustness

- [x] 3.1 `PillGroup` renders selected-but-unknown values as removable pills so
      a removed vocabulary value in an old bookmark/saved search is not a stuck,
      invisible filter.

## 4. Verification

- [x] 4.1 `go build/vet/test ./...` green.
- [x] 4.2 `svelte-check` clean; web production build succeeds.
- [x] 4.3 Post-deploy: `cmd/backfill-derive` + `make reindex`, then confirm the
      region facet and live skills/countries selects on staging/prod. *(Deferred
      to rollout — needs the running stack.)*
