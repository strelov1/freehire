## 1. Data layer — `web/src/lib/collections.ts`

- [x] 1.1 Add a `FilterCollection` type `{ slug, title, description, params: Record<string, string | string[]> }`
- [x] 1.2 Add `FILTER_COLLECTIONS: FilterCollection[]` seeded with `remote-worldwide` (`{ work_mode: 'remote', regions: 'global' }`), with title "Remote Worldwide" and a description
- [x] 1.3 Add a `toQuery(params)` helper that builds a query string / `URLSearchParams`, expanding list values into repeated keys (OR semantics)

## 2. Server load — `web/src/routes/collections/+page.server.ts`

- [x] 2.1 Keep the existing company-collection facet-distribution count call unchanged
- [x] 2.2 Fetch each filter collection's count via `searchJobs(toQuery(params), 0, 0).total` under `Promise.all`, each wrapped in try/catch (decorative → degrade to no count)
- [x] 2.3 Return one normalized `cards` view-model array `{ title, description, href, count }` (keyed by `href`, which is unique across both kinds) combining filter cards (`href=/jobs?<query>`, first) then company cards (`href=/jobs?collections=<slug>`)

## 3. Hub render — `web/src/routes/collections/+page.svelte`

- [x] 3.1 Render the grid from the unified `cards` list using `card.href` and `card.count`, removing the inline `/jobs?collections=` href construction
- [x] 3.2 Update the hub subtitle copy from "Curated groups of companies" to reflect roles + companies

## 4. Verify

- [x] 4.1 `svelte-check` passes (types on `params` / `cards`) and `npm run build` succeeds
- [x] 4.2 Manually load `/collections`: the `remote-worldwide` card appears with a count and links to `/jobs?work_mode=remote&regions=global`; existing company-collection cards and counts are unchanged
