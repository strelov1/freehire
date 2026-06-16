## Why

Job detail pages (`/jobs/:slug`) emit Open Graph / Twitter tags but carry **no
`og:image`**, and the Twitter card is the small `summary` type — so links to
vacancies have no visual preview when shared on social platforms, chat apps, or
search snippets. A per-job preview image (the idea behind hirify's
`api.hirify.me/og-media/<id>.jpg`, but in the freehire brand and data) makes
shared links legible and on-brand.

The full design is in `docs/superpowers/specs/2026-06-16-job-og-images-design.md`
(source of truth for the layout and degradation rules).

## What Changes

- Add a SvelteKit server endpoint `GET /jobs/:slug/og.png` that renders a
  1200×630 PNG preview for that job, on demand, in the freehire brand.
- Render via the `satori` (HTML → SVG) + `@resvg/resvg-js` (SVG → PNG) stack in
  the web Node tier; add a server-only `web/src/lib/server/og/` module
  (`card.ts` pure HTML builder + degradation rules, `logo.ts` logo.dev → data-URI
  with monogram fallback, `render.ts` rasterizer) and bundle Inter font files.
- Populate the card from the served `Job` contract (title, company, work mode,
  seniority, salary via the existing `formatSalary`, top skills); degrade
  gracefully when fields are missing (the common case in this catalogue).
- Emit `og:image` (1200×630) and switch the job page to the
  `summary_large_image` Twitter card by giving `Seo.svelte` an optional `image`
  prop and passing the absolute `og.png` URL from the job-detail page.
- New runtime dependencies: `satori`, `satori-html`, `@resvg/resvg-js`.

## Capabilities

### New Capabilities
- `job-og-images`: on-demand rendering of a per-job 1200×630 Open Graph preview
  image, served from a dedicated endpoint, populated from the job's served wire
  fields with defined graceful-degradation behaviour for missing data and logos.

### Modified Capabilities
- `web-ssr-seo`: the per-route document-metadata requirement extends so the
  job-detail page emits an `og:image` (with dimensions and alt) and uses the
  `summary_large_image` Twitter card when a preview image exists.

## Impact

- **Code (web tier only):** new `web/src/routes/jobs/[slug]/og.png/+server.ts`;
  new `web/src/lib/server/og/` module + bundled `Inter-*.ttf`; edits to
  `web/src/lib/components/Seo.svelte` and `web/src/routes/jobs/[slug]/+page.svelte`.
- **Dependencies:** `satori`, `satori-html`, `@resvg/resvg-js` added to `web/`.
  `@resvg/resvg-js` ships a native binary — the web `Dockerfile` (Node/Alpine)
  must resolve the correct musl prebuilt.
- **No backend, DB, or API changes.** The endpoint reuses the existing
  `serverApi(...).getJob(slug)` read path.
- **Caching:** endpoint sets `Cache-Control: public, max-age=3600,
  stale-while-revalidate=86400`. No new persistent storage.
- **Verification:** e2e-driven (Playwright on the endpoint); no new JS unit
  runner (project convention).
