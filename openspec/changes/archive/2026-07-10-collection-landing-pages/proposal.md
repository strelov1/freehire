## Why

Curated collections (`Y Combinator`, `AI Companies`, `Remote Worldwide`, …) are
high-intent search targets with ready-made copy, but they have no indexable URL:
every collection card links to `/jobs?collections=<slug>`, and the `/jobs` list
hard-codes its canonical to the bare `/jobs`, so Google collapses all collections
into one page. The curated landing content cannot rank. This change gives each
collection its own server-rendered, self-canonical landing page.

## What Changes

- Add a server-rendered route `/collections/:slug` that renders a collection's
  job feed with a collection-specific `<title>`, `<h1>`, `<meta name="description">`,
  and a self-canonical URL. Unknown slug → 404.
- Reuse the existing `JobsView` `scope` mechanism (the same one company pages use
  for `company_slug`) to pin the collection's facet params — no new filtering code.
- Point the `/collections` hub cards at `/collections/:slug` instead of `/jobs?…`.
- Expand `FILTER_COLLECTIONS` with regional remote landings (e.g. Remote Latam,
  Remote Brasil, Remote US) and language/framework landings (the "<lang> jobs"
  search pattern: Python, Go, Rust, Ruby, Node.js, React, … mapping to the `skills`
  facet) — additive data entries, each validated against the live facet vocabulary
  and shipped only when it has a healthy, non-empty count.
- List the `/collections` hub, each collection landing, and `/for-companies` in
  the sitemap.
- Add a visible `<h1>` to the `/jobs` and `/companies` list pages (currently
  missing).

## Capabilities

### New Capabilities
<!-- none: this surfaces existing collections through the existing SSR/SEO capability -->

### Modified Capabilities
- `web-ssr-seo`: add indexable collection landing pages (per-collection metadata +
  visible `<h1>`), extend the sitemap to enumerate the collection pages and the
  `/collections` hub, and require a visible `<h1>` on the public list pages.

## Impact

- **web/** — new route `web/src/routes/collections/[slug]/` (server load + page);
  `collections/+page.server.ts` (card hrefs); `lib/collections.ts` (new
  filter-collection entries + a unified slug→collection lookup); `lib/sitemap.ts`
  (`STATIC_PATHS` + collection URLs); `JobsView`/`CompaniesView` (`<h1>`).
- No backend changes: the search API already accepts `collections`, `regions`,
  `countries`, and `work_mode` facet params.
