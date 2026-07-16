## Why

The Insights API (`market-insights`) exposes rich aggregate market data but has no
human-facing surface — only JSON. freehire's proven growth channel is organic
search (collection landing pages, programmatic SEO). Turning the insights data into
crawlable, keyword-targeted landing pages ("Backend developer salaries", "Most
in-demand backend skills") drives organic traffic to freehire and reuses data that
already exists. This is the follow-up the Trends & Insights work was sequenced
before.

## What Changes

- Add server-rendered **insights landing pages** under `/insights`:
  - `/insights` — a hub linking every covered category and insight type.
  - `/insights/salary/[category]` — salary bands by seniority for a role category.
  - `/insights/skills/[category]` — most in-demand skills in a category, with growth.
  - `/insights/roles/[category]` — most-hiring roles (seniorities) in a category.
- Pages are **SSR-live**: `server-load` reads the internal Insights API, with
  `Cache-Control: s-maxage=3600` so CDN/nginx absorbs repeat crawls.
- **Standard SEO content** per page: a data-driven auto-generated intro, the
  ranking table / salary bands, an "updated" date, internal links (to `/jobs`
  filtered views, sibling categories, and the other insight types for the same
  category), canonical/meta/OG, and JSON-LD (BreadcrumbList + Dataset).
- A **quality gate**: only categories whose data clears a threshold (enough open
  jobs / a salary band above the sample floor) get a page and a sitemap entry;
  thin categories are excluded (no thin-content pages).
- New **`sitemap-insights.xml`** shard, wired into the existing sitemap index.
- **Two small Insights API additions** to serve the pages that today's endpoints
  can't in one call: an optional `category` filter on `GET /insights/roles`
  (return the category's seniorities), and an all-seniority salary read for a
  category.

Out of scope for v1 (explicit): geo/country page matrix, editorial prose / FAQ,
charts, and dedicated velocity pages.

## Capabilities

### New Capabilities
- `seo-insights-pages`: public server-rendered landing pages over the insights
  data — hub + per-category salary/skills/roles pages — with SEO content, a
  data-quality gate, internal linking, and a sitemap shard.

### Modified Capabilities
- `market-insights`: add an optional `category` scope to the roles read, and a
  per-category all-seniority salary read, so the landing pages can be served from
  single API calls.

## Impact

- **Backend**: extend `internal/db/queries/insights.sql` read queries + handler
  params for the roles `category` filter and the category salary read (sqlc
  regen); no rollup/schema change (the rollups already hold the rows).
- **Frontend**: new SvelteKit routes under `web/src/routes/insights/**`
  (`+page.server.ts` + `+page.svelte`), a shared insights page component, and the
  category quality-gate helper; new `web/src/routes/sitemap-insights.xml` +
  registration in the sitemap index; `docs/API.md` + `web/static/openapi.yaml`
  updated for the two new read shapes.
- No changes to the rollup worker, migrations, auth, or ingest.
