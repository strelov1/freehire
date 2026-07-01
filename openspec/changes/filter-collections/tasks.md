## 1. Data layer — `web/src/lib/collections.ts`

- [ ] 1.1 Add a `FilterCollection` type `{ slug, title, description, params: Record<string, string | string[]> }`
- [ ] 1.2 Add `FILTER_COLLECTIONS: FilterCollection[]` seeded with `remote-worldwide` (`{ work_mode: 'remote', regions: 'global' }`), with title "Remote Worldwide" and a description
- [ ] 1.3 Add a `toQuery(params)` helper that builds a query string / `URLSearchParams`, expanding list values into repeated keys (OR semantics)

## 2. Server load — `web/src/routes/collections/+page.server.ts`

- [ ] 2.1 Keep the existing company-collection facet-distribution count call unchanged
- [ ] 2.2 Fetch each filter collection's count via `searchJobs(toQuery(params), 0, 0).total` under `Promise.all`, each wrapped in try/catch (decorative → degrade to no count)
- [ ] 2.3 Return one normalized `cards` view-model array `{ slug, title, description, href, count }` combining filter cards (`href=/jobs?<query>`, first) then company cards (`href=/jobs?collections=<slug>`)

## 3. Hub render — `web/src/routes/collections/+page.svelte`

- [ ] 3.1 Render the grid from the unified `cards` list using `card.href` and `card.count`, removing the inline `/jobs?collections=` href construction
- [ ] 3.2 Update the hub subtitle copy from "Curated groups of companies" to reflect roles + companies

## 4. Verify

- [ ] 4.1 `svelte-check` passes (types on `params` / `cards`) and `npm run build` succeeds
- [ ] 4.2 Manually load `/collections`: the `remote-worldwide` card appears with a count and links to `/jobs?work_mode=remote&regions=global`; existing company-collection cards and counts are unchanged
