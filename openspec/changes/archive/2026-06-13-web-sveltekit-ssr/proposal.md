## Why

The public job pages are effectively invisible to search engines and link
previews. `web/` is a client-only Vite + Svelte SPA: every URL (including
`/jobs/:slug`, `/robots.txt`, `/sitemap.xml`) returns the same empty shell
(`<div id="app"></div>`, static `<title>freehire</title>`), and all content is
painted client-side after the JS bundle loads and calls `/api`. Googlebot may
eventually render it via its JS pass, but non-Google crawlers, social/chat link
previews, and job aggregators see nothing — and for a job board, organic and
Google Jobs traffic to `/jobs/:slug` is the point of the product.

## What Changes

- **BREAKING (deploy topology):** Migrate `web/` from a static Vite + Svelte SPA
  to **SvelteKit with SSR** (`adapter-node`). The frontend container becomes a
  Node server instead of static files behind nginx; nginx (and the dev proxy)
  front the Node app rather than serving `index.html` directly.
- Public read pages (`/`, `/jobs/:slug`, `/companies`, `/companies/:slug`) are
  **server-rendered**: their content is present in the initial HTML response and
  hydrates on the client. Authenticated/interactive surfaces (`/my/*`,
  API-keys, theme, filters) keep their current client behavior.
- Per-route document `<head>`: a real, job-specific `<title>`, `<meta name="description">`,
  canonical URL, and Open Graph / Twitter Card tags — replacing the single
  static `freehire` title.
- `JobPosting` **JSON-LD** structured data on `/jobs/:slug` (and `Organization`
  on company pages) so the postings are eligible for Google Jobs.
- A real **`/robots.txt`** and a dynamically generated **`/sitemap.xml`** that
  enumerates indexable job and company pages from the API/DB.
- Preserve all existing SPA behavior and routes (same paths, same auth cookie
  flow, same filters/URL-sync, themes, load states) through the migration — this
  is a rendering/infrastructure change, not a product redesign.

## Capabilities

### New Capabilities

- `web-ssr-seo`: Public pages are server-rendered with per-route metadata,
  `JobPosting`/`Organization` JSON-LD, a real `robots.txt`, and a generated
  `sitemap.xml`, so crawlers and link-preview bots see full content in the
  initial HTML response.

### Modified Capabilities

- `web-frontend`: The frontend's rendering model changes from a client-only SPA
  to SvelteKit SSR with hydration. Public read views (jobs list, job detail,
  companies list, company detail) SHALL be server-rendered so their content is
  in the initial HTML; all existing view behavior (fields shown, pagination,
  search, closed state, async states) is preserved.

## Impact

- **`web/`**: project restructured to SvelteKit (`src/routes/**`, `+page.svelte`,
  `+page.server.ts`/`+layout`), replacing `App.svelte` + the hand-rolled
  `router.svelte.ts`. Existing components/`api.ts`/`types.ts`/stores are reused;
  data loading moves into SvelteKit `load` functions.
- **Build/deploy**: `web/Dockerfile`, `web/nginx.conf`, and the ops deploy path
  change from "build static + nginx" to "build + run a Node server"
  (`adapter-node`). `web/vite.config.ts` dev proxy is replaced by SvelteKit's
  dev server config.
- **API/data**: the server-side `load` and the sitemap need a way to fetch jobs
  and companies (existing public read endpoints; sitemap may need a lightweight
  slug/updated-at listing). Same-origin auth cookie flow must keep working
  through SSR (cookie forwarded on server-side fetches).
- **Out of scope**: backend job/company API shape, the existing CORS requirement
  (unchanged), enrichment/ingest, and any product/UX redesign.
