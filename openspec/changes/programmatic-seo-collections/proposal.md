## Why

The `/collections/[slug]` programmatic-SEO pages (both filter collections like
`react`/`remote-europe` and company collections like `yc`/`bigtech`, unified
behind one route) carry static titles that never surface the live job count,
so search snippets look generic. They're also invisible from the
highest-traffic page type on the site — job postings link to none of them —
so Google has to discover and re-crawl them independently. The filter-collection
list itself is also thinner than the realistic set of high-intent search
patterns ("react jobs", "remote jobs germany") it could cover.

## What Changes

- `/collections/[slug]` (and its hub cards) interpolate the live, exact job
  count into `<title>`, `<h1>`, and the OG title — e.g.
  `"1,234 React Jobs · freehire"` — for both collection kinds, since they
  render through the same route and already have the count in hand at SSR
  time (`meta.total` from the existing job-list fetch).
- `FILTER_COLLECTIONS` (`web/src/lib/collections.ts`) grows with more
  high-intent country/skill/role combinations, each verified to have a
  healthy live count before being added — a data addition under the
  registry's existing contract, not a behavior change.
- A new "see also" block on the job detail page (`/jobs/[slug]`) links to a
  handful of existing `/collections/[slug]` pages, matched from the viewed
  job's own facets (role, region, skills) and from its `collections` field
  (company membership, e.g. `yc`) — padded with popular collections when a
  job has few matches. Every link targets an already-existing collection
  slug; no thin or empty pages are ever linked.

## Capabilities

### New Capabilities
- `related-collections`: the job-detail-page "see also" block — matching a
  viewed job's own facets and company collections against the two existing
  collection registries to produce a bounded, deduped, always-full list of
  internal links into `/collections/[slug]`.

### Modified Capabilities
- `web-ssr-seo`: the "Collection landing pages are indexable" requirement's
  `<title>`/`<h1>`/OG-title format changes from a static collection name to
  include the collection's live, exact job count.

## Impact

- Frontend only (SvelteKit, `web/`): no new backend endpoint — every count
  needed is already returned by the existing job-list SSR fetch on
  `/collections/[slug]`.
- Touches `web/src/routes/collections/[slug]/+page.svelte` (title/H1/OG),
  `web/src/lib/collections.ts` (expanded `FILTER_COLLECTIONS`, new matching
  helper for the see-also block), and `web/src/routes/jobs/[slug]` (new
  block).
- No change to `internal/collections`, the `jobs.collections` propagation, or
  any existing backend facet/search behavior.
