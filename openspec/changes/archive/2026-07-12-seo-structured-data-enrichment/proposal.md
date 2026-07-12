## Why

The company-detail `Organization` JSON-LD is a bare `{ name, url }` stub even
though the company row already carries rich `company_info` (logo, description,
website, LinkedIn, industries, founding year, employee count, HQ country) — so
search and AI engines cannot recognize the company as an entity (no logo in the
knowledge panel, weak citation signal). Separately, the collection landing pages
(`/collections/:slug` — our primary SEO landings like "React jobs") emit only a
`BreadcrumbList` and no listing structured data, so engines do not see the page
as a curated job collection.

## What Changes

- Enrich the company `Organization` JSON-LD with the fields already present on
  the company: `logo`, `description`, `sameAs` (website + LinkedIn),
  `foundingDate`, `numberOfEmployees`, and `address.addressCountry`. Every field
  is conditional — emitted only when the source provides it.
- Add `CollectionPage` JSON-LD wrapping an `ItemList` of the first page of jobs
  to `/collections/:slug`. Each `ListItem` is a summary entry (`position`,
  `name`, `url`) pointing at the job-detail page — not an embedded full
  `JobPosting` (Google's recommended list-page shape).

Out of scope (deliberately deferred): per-collection FAQ, `ItemList` on company
pages, meta-description rewrites, and Core Web Vitals work.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `web-ssr-seo`: the "JobPosting structured data" requirement's `Organization`
  clause is strengthened (company `Organization` carries its known company-info
  facts), and a new requirement adds `CollectionPage` + `ItemList` structured
  data to collection landing pages.

## Impact

- `web/src/lib/seo.ts` — `organizationJsonLd` gains conditional fields; a new
  `collectionPageJsonLd` helper is added (pure functions).
- `web/src/routes/collections/[slug]/+page.svelte` — composes the new helper
  into its existing `jsonLdScript([...])` block using already-loaded SSR data.
- `web/src/lib/seo.test.ts` — new vitest unit coverage for both helpers.
- No backend, API, schema, or data changes. No new dependencies.
