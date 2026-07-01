## Why

`GET /sitemap.xml` is a single file hard-capped at 5,000 jobs + 5,000 companies,
but the catalogue holds ~2.5M open jobs and ~88k companies — so ~95% of job
detail pages are absent from the sitemap and are discoverable only by internal
link-following, which crawlers do slowly and incompletely. A sitemap index that
enumerates the full open catalogue lets search engines discover every indexable
page directly.

## What Changes

- **BREAKING** `GET /sitemap.xml` stops being a `<urlset>` and becomes a
  `<sitemapindex>` that references sub-sitemaps (static pages, job chunks,
  company chunks). Existing crawlers re-read the index and follow the children;
  no external contract beyond "valid sitemap at /sitemap.xml" is broken.
- New sub-sitemap routes serve `<urlset>` chunks of ≤50,000 URLs each
  (sitemap-protocol limit), covering the **full** set of open jobs and companies.
- Sub-sitemaps are addressed by a **keyset cursor** (last id / last slug of the
  previous chunk), not a page number, so each chunk is a bounded index scan
  rather than a deep `OFFSET` over millions of rows.
- New slim backend endpoints return only the fields the sitemap needs
  (`public_slug` + `updated_at` for jobs; `slug` + `updated_at` for companies)
  via keyset pagination, plus chunk-boundary cursors for building the index —
  so the sitemap never drags wide rows or the search engine into the request.
- The old 5,000-row caps and their truncation warnings are removed.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `web-ssr-seo`: the "robots.txt and sitemap" requirement changes — the sitemap
  becomes a sitemap index over sub-sitemaps that enumerate the entire open
  catalogue (no cap), served via keyset-paginated slim endpoints.

## Impact

- **Backend (Go/Fiber + sqlc):** new read-only queries and handlers for
  keyset-paginated job/company sitemap slices and chunk-boundary cursors;
  new routes under `/api/v1` registered before the `/:slug` catch-alls.
- **Frontend (SvelteKit):** `/sitemap.xml` rewritten as an index; new
  `+server.ts` routes for the static-pages, job, and company sub-sitemaps; new
  slim API-client methods.
- **No schema/migration changes** — reads existing `jobs`/`companies` columns
  and the primary-key / slug ordering. Sitemap responses stay HTTP-cached.
- Crawl surface grows from ~10k URLs to the full open catalogue (~2.6M URLs).
