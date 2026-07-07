## 1. Collection registry lookup

- [x] 1.1 Add a unified `collectionBySlug(slug)` helper in `lib/collections.ts` that resolves a slug to `{ title, description, params }`, checking `FILTER_COLLECTIONS` then `COLLECTIONS` (`{ collections: slug }`); returns undefined for unknown slugs
- [x] 1.2 Add a `collectionSlugs()` / exported list of all collection slugs for the sitemap, sourced from the same two registries
- [x] 1.3 Unit-test the lookup: known filter slug, known company slug, unknown slug, and slug uniqueness across the two sets

## 2. Collection landing route

- [x] 2.1 Create `web/src/routes/collections/[slug]/+page.server.ts`: resolve the slug via `collectionBySlug`, `error(404)` when unknown, thread `url.searchParams` and set the collection's scope params on top, fetch `initial = searchJobs(facets, 20, 0)`, return `{ collection, initial }`
- [x] 2.2 Create `web/src/routes/collections/[slug]/+page.svelte`: `<Seo>` with self-canonical `/collections/:slug`, template `<title>` (`"<title> jobs · freehire"`) and description; visible `<h1>` (`"<title> jobs"`) + intro copy; BreadcrumbList JSON-LD; render `<JobsView {initial} scope={params} excludeFacets={collectionFacetKeys} />`
- [x] 2.3 Verify SSR: the landing HTML contains the `<h1>`, the job rows, and a self-referential canonical (not `/jobs`); an unknown slug returns 404; a filtered URL (`?work_mode=remote`) server-renders the filtered subset

## 3. Hub links

- [x] 3.1 In `collections/+page.server.ts`, carry each card's `slug` and set `href` to `/collections/:slug` (drop the `/jobs?…` href); keep the count logic unchanged
- [x] 3.2 Confirm `collections/+page.svelte` links resolve to the landing pages

## 4. Regional remote collections (data)

- [x] 4.1 Add regional remote entries to `FILTER_COLLECTIONS` with validated facet params: Remote Latam (`regions: latam`), Remote Brasil (`countries: br`), Remote US (`countries: us`), each with `work_mode: remote`
- [x] 4.2 Verify each new collection returns a healthy, non-empty count against the live search API; drop or defer any that are thin
- [x] 4.3 (Optional) add Remote Europe (`regions: eu`) / Remote APAC (`regions: apac`) if counts warrant — both shipped (29k / 15k live counts)
- [x] 4.4 Add language/framework collections (the "<lang> jobs" pattern) mapping to the `skills` facet, using exact skilltag canonicals (`go`/`nodejs`/`cpp`/`csharp`, not `golang`/`node`/`c++`/`c#`); 16 languages + 8 frameworks, each count-verified against the live `/jobs/facets` distribution

## 5. Sitemap

- [x] 5.1 Add `/collections` and `/for-companies` to `STATIC_PATHS` in `lib/sitemap.ts`
- [x] 5.2 Append one `/collections/:slug` per collection slug to the static-pages sub-sitemap
- [x] 5.3 Verify `GET /sitemap-pages.xml` lists the hub and every collection landing

## 6. List-page headings

- [x] 6.1 Add a single semantic `<h1>` to the `/jobs` route page ("Tech jobs") — placed at route level (not in `JobsView`) so embedded/scoped uses keep their own single `<h1>`
- [x] 6.2 Add a single semantic `<h1>` to the `/companies` route page ("Companies hiring in tech") — same route-level placement rationale
- [x] 6.3 Verify `/jobs` and `/companies` each render exactly one `<h1>`

## 7. Verification

- [x] 7.1 `npm run check` (svelte-check) and lint pass
- [x] 7.2 Validate a landing page's JSON-LD / metadata and canonical in the rendered HTML
- [x] 7.3 `openspec validate collection-landing-pages --strict` passes
