## 1. Collection registry lookup

- [ ] 1.1 Add a unified `collectionBySlug(slug)` helper in `lib/collections.ts` that resolves a slug to `{ title, description, params }`, checking `FILTER_COLLECTIONS` then `COLLECTIONS` (`{ collections: slug }`); returns undefined for unknown slugs
- [ ] 1.2 Add a `collectionSlugs()` / exported list of all collection slugs for the sitemap, sourced from the same two registries
- [ ] 1.3 Unit-test the lookup: known filter slug, known company slug, unknown slug, and slug uniqueness across the two sets

## 2. Collection landing route

- [ ] 2.1 Create `web/src/routes/collections/[slug]/+page.server.ts`: resolve the slug via `collectionBySlug`, `error(404)` when unknown, fetch `initial = searchJobs(params, 20, 0)`, return `{ collection, initial }`
- [ ] 2.2 Create `web/src/routes/collections/[slug]/+page.svelte`: `<Seo>` with self-canonical `/collections/:slug`, template `<title>` (`"<title> jobs · freehire"`) and description; visible `<h1>` (`"<title> jobs"`) + intro copy; render `<JobsView {initial} scope={params} excludeFacets={collectionFacetKeys} />`
- [ ] 2.3 Verify SSR: the landing HTML contains the `<h1>`, the job rows, and a self-referential canonical (not `/jobs`); an unknown slug returns 404

## 3. Hub links

- [ ] 3.1 In `collections/+page.server.ts`, carry each card's `slug` and set `href` to `/collections/:slug` (drop the `/jobs?…` href); keep the count logic unchanged
- [ ] 3.2 Confirm `collections/+page.svelte` links resolve to the landing pages

## 4. Regional remote collections (data)

- [ ] 4.1 Add regional remote entries to `FILTER_COLLECTIONS` with validated facet params: Remote Latam (`regions: latam`), Remote Brasil (`countries: br`), Remote US (`countries: us`), each with `work_mode: remote`
- [ ] 4.2 Verify each new collection returns a healthy, non-empty count against the live search API; drop or defer any that are thin
- [ ] 4.3 (Optional) add Remote Europe (`regions: eu`) / Remote APAC (`regions: apac`) if counts warrant

## 5. Sitemap

- [ ] 5.1 Add `/collections` and `/for-companies` to `STATIC_PATHS` in `lib/sitemap.ts`
- [ ] 5.2 Append one `/collections/:slug` per collection slug to the static-pages sub-sitemap
- [ ] 5.3 Verify `GET /sitemap-pages.xml` lists the hub and every collection landing

## 6. List-page headings

- [ ] 6.1 Add a single semantic `<h1>` to `JobsView` (e.g. "Tech jobs"), styled to match existing headers
- [ ] 6.2 Add a single semantic `<h1>` to `CompaniesView` (e.g. "Companies hiring in tech")
- [ ] 6.3 Verify `/jobs` and `/companies` each render exactly one `<h1>`

## 7. Verification

- [ ] 7.1 `npm run check` (svelte-check) and lint pass
- [ ] 7.2 Validate a landing page's JSON-LD / metadata and canonical in the rendered HTML
- [ ] 7.3 `openspec validate collection-landing-pages --strict` passes
