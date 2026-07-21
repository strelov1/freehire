## 1. Generalize the scope-summary helper

- [x] 1.1 Extend `web/src/lib/headerScope.test.ts`: keep the existing jobs cases green via the default spec, and add company-spec cases — `summarizeScope(store, COMPANIES_SCOPE)` with `remote_regions=[eu]` → `{ icon: 'globe', label: 'Europe' }`; `regions=[eu]`+`remote_regions=[uk]` → `{ icon: 'globe', label: 'Europe +1' }` (geo order regions then remote_regions); company spec ignores `work_mode` (a selected `work_mode` does not change the label/icon).
- [x] 1.2 Refactor `web/src/lib/headerScope.ts`: `summarizeScope(store, spec = JOBS_SCOPE)` where `spec: { format?: string; geo: string[] }`. Export `JOBS_SCOPE = { format: 'work_mode', geo: ['regions','countries','cities'] }` and `COMPANIES_SCOPE = { geo: ['regions','remote_regions'] }`. Icon from `spec.format`'s first selected work mode (else `'globe'`). Geo label per param: `countries`→`countryLabel`, `cities`→raw, else `REGION_LABELS[v] ?? v`. Same `+N` roll-up.
- [x] 1.3 Run `npx vitest run src/lib/headerScope.test.ts` — all pass.

## 2. Bridge variant

- [x] 2.1 In `web/src/lib/listSearch.svelte.ts`, add `variant: 'jobs' | 'companies'` to the `filterScope` object type (alongside `store`/`counts`); document that it selects the popover body.

## 3. Register the two list variants

- [x] 3.1 In `web/src/lib/components/JobsView.svelte`, add `variant: 'jobs'` to the `filterScope` it registers (store/counts unchanged).
- [x] 3.2 In `web/src/lib/components/CompaniesView.svelte`, change `setListSearchTarget(filters)` to register an adapter `{ get value(){return filters.value}, setQuery:(q)=>filters.setQuery(q), filterScope: { store: filters, counts: () => null, variant: 'companies' } }` (CompaniesView fetches no facet counts, so counts is null — pills render countless, null-safe). Leave the cleanup `setListSearchTarget(null)` as-is.

## 4. Variant-aware popover

- [x] 4.1 In `web/src/lib/components/HeaderLocationFilter.svelte`, add prop `variant: 'jobs' | 'companies' | 'launcher'` (default `'jobs'`). Compute the trigger summary with `summarizeScope(store, variant === 'companies' ? COMPANIES_SCOPE : JOBS_SCOPE)`; the `jobs` body is unchanged.
- [x] 4.2 Add the **companies** body: a `Region` pill row (`REGION_OPTIONS` → `store.cycle('regions', v)`) and a `Remote hiring` pill row (`REGION_OPTIONS` → `store.cycle('remote_regions', v)`), each pill styled with `pillClass`/`pillTitle` and state from `store.facet(...)`. `Clear all` clears `regions` + `remote_regions`. No work-format / LocationPane in this mode.
- [x] 4.3 Run `npx svelte-check` — no new errors; visually verify jobs mode is unchanged and companies mode renders on `/companies`.

## 5. Launcher mode (listless pages)

- [x] 5.1 Add the **launcher** body to `HeaderLocationFilter.svelte` (no store): a `Work format` pill row and a flat `Region` pill row (`REGION_OPTIONS`); each pill's `onclick` runs `goto(\`${resolve('/')}?${param}=${value}\`)` (mirroring HeaderSearch's `runFullSearch` eslint-disable pattern) and closes the popover. Trigger label is the static neutral `Location`.
- [x] 5.2 In `web/src/lib/components/HeaderSearch.svelte`, render `<HeaderLocationFilter variant="launcher" />` + a divider before the `<Search>` icon inside the box (so listless pages get the trigger). Text search + dropdown unchanged.

## 6. Wire the variant into the list search box

- [x] 6.1 In `web/src/lib/components/HeaderListSearch.svelte`, pass `variant={target.filterScope.variant}` to `<HeaderLocationFilter>` (store/counts as today).

## 7. Verify end-to-end

- [x] 7.1 Dev server: (a) jobs feed `/` unchanged (work format + location, live, URL); (b) `/companies` shows Region + Remote-hiring, selecting filters the company list + URL; (c) a job-detail page shows the launcher trigger, picking a region navigates to `/jobs?regions=…`.
- [x] 7.2 Run `npx vitest run`, `npx svelte-check`; changed files clean on `eslint`.
