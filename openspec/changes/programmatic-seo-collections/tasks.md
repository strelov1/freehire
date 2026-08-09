## 1. Live job count in collection title/H1/OG

- [ ] 1.1 In `web/src/routes/collections/[slug]/+page.svelte`, interpolate the
      already-fetched `meta.total` into `<title>`, `<h1>`, and the OG title
      (exact count, e.g. `"1,234 React Jobs · freehire"`), for both filter
      and company collections (single shared route/component).
- [ ] 1.2 Test: rendered `<title>`/`<h1>`/OG-title match a fixture
      `meta.total` exactly, covering one filter-collection slug (e.g. `react`)
      and one company-collection slug (e.g. `yc`).

## 2. Expand `FILTER_COLLECTIONS`

- [ ] 2.1 For each candidate new entry (additional country-level remote
      landings, additional skill landings, additional role landings — see
      design.md's Context), verify a healthy non-empty live count via
      `searchJobs(params, 0, 0).total` before adding it.
- [ ] 2.2 Add the verified entries to `FILTER_COLLECTIONS` in
      `web/src/lib/collections.ts`, following the existing entry shape and
      section grouping.

## 3. See-also matching logic

- [ ] 3.1 Implement a pure function in `web/src/lib/collections.ts` (or a
      sibling module) that takes a job's facets (role, region, skills) and its
      `collections` field, and returns an ordered, deduped list of collection
      slugs: Source A (facet match against `FILTER_COLLECTIONS`) before
      Source B (`collections` match against the company-collection registry),
      padded with a fixed popular-collections fallback up to a target size,
      never exceeding the available pool and never inventing a non-existent
      slug.
- [ ] 3.2 Unit tests for the matching function: Source A match, Source B
      match, both together (ordering), dedupe across sources and fallback, a
      job with zero matches (fallback-only), and a pool smaller than the
      target size.

## 4. See-also block on the job detail page

- [ ] 4.1 Add a "see also" block to the job detail page
      (`web/src/routes/jobs/[slug]`) rendering the slugs from the section-3
      matching function as links to their `/collections/[slug]` pages, using
      data the job page already loads (no new fetch).
- [ ] 4.2 Test: the block renders the expected links for a job fixture with
      facet matches, a job fixture with only company-collection matches, and
      a job fixture with no matches (fallback list).
- [ ] 4.3 Manual verification: load a real job page locally and confirm the
      block shows correct, working links and the linked collection pages show
      the new live-count title.
