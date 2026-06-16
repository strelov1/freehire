## Context

Job detail pages are SSR'd SvelteKit routes (`web/`, adapter-node behind nginx).
`Seo.svelte` emits per-route `<head>` metadata but no `og:image`; the Twitter
card is `summary`. The full visual design — direction "A Editorial", light,
1200×630 — and the degradation rules were brainstormed and recorded in
`docs/superpowers/specs/2026-06-16-job-og-images-design.md`. This document is the
technical companion: it records the architectural choices, not the pixel layout.

The served `Job` wire contract already carries everything the card needs
(`title`, `company`, `work_mode`, `skills`, `enrichment.seniority`, salary
fields). The catalogue is large (~390k jobs) and most jobs lack salary and many
lack a resolvable logo, so degradation is a first-class concern, not an edge.

## Goals / Non-Goals

**Goals:**
- A per-job 1200×630 PNG preview, on demand, in the freehire brand.
- Card design and degradation live in the web tier, in TypeScript, reusing the
  generated `Job` contract and the existing `formatSalary`.
- `og:image` + `summary_large_image` on the job page; other routes unaffected.

**Non-Goals:**
- OG images for company or list pages (same module extends later).
- Dark-theme image variants (OG images are static; platforms ignore theme).
- Pre-generation/storage, CDN or nginx caching config (ops-tier, separate repo).
- A new JS unit-test runner in `web/` (project convention).

## Decisions

### Render on demand in SvelteKit with satori + resvg
Endpoint `web/src/routes/jobs/[slug]/og.png/+server.ts` (same routing idiom as the
existing `sitemap.xml/+server.ts`, `robots.txt/+server.ts`). It fetches the job
via `serverApi(fetch).getJob(slug)`, renders, and returns `image/png`.
- **Why:** keeps brand/design/contract in the web tier; `satori` + `@resvg/resvg-js`
  is the proven `@vercel/og` pipeline; iterating the visual is TypeScript.
- **Alternatives:** render in Go (no satori-equivalent; hand-rolled text layout,
  brand drift) — rejected. Pre-generate + store (~390k images + invalidation on
  every edit/enrichment/close) — premature; noted as a future seam.

### Module split keeps the layout pure
`web/src/lib/server/og/` (server-only):
- `card.ts` — **pure** `buildCard(job, { logo }) => string`: the card HTML and all
  degradation logic. Sync, no I/O — so it is reviewable/verifiable in isolation
  even without a unit runner.
- `logo.ts` — `resolveLogo(company) => Promise<string | null>`: the async logo.dev
  fetch (satori does not fetch remote images itself), bounded by a timeout,
  returns a `data:` URI or `null`.
- `render.ts` — `renderOgPng(job) => Promise<Uint8Array>`: resolve logo →
  `buildCard` → `satori-html` → `satori` (SVG) → `@resvg/resvg-js` (PNG). Loads
  Inter fonts once at module init via `read()` from `$app/server`.
- `fonts/Inter-{Regular,SemiBold,Bold}.ttf` — bundled. satori needs real font
  files; the site's `system-ui` stack is not usable, and Inter is the standard
  stand-in. `font-weight: 500` maps to SemiBold.

### Logo via logo.dev, monogram fallback
Reuse the publishable logo.dev token the SPA already uses (extract the literal
from `CompanyLogo.svelte` into a shared `$lib` constant rather than duplicating
it). Fetch `name/<company>?token=…&fallback=404` server-side; on `200` embed the
bytes as a `data:` URI; on `404`/timeout/error fall back to a monogram tile. Logo
failure is never a render failure.

### Optional `image` prop on Seo.svelte
`Seo.svelte` gains an optional `image?: string` (absolute URL). When set it emits
`og:image` (+ width/height/alt) and switches `twitter:card` to
`summary_large_image` with `twitter:image`; when absent, behaviour is unchanged.
The job page passes `${origin}/jobs/${slug}/og.png` (og:image must be absolute;
`origin` is already derived from `page.url.origin`).

### Caching
`Cache-Control: public, max-age=3600, stale-while-revalidate=86400`. Job content
changes (enrichment, close) so the URL is not immortal, but crawlers refetch
rarely. No in-process LRU and no nginx proxy_cache in v1 — noted as seams.

## Risks / Trade-offs

- **[`@resvg/resvg-js` native binary in Docker]** → the web image is Node/Alpine
  (musl); verify the correct prebuilt resolves in the `Dockerfile` build, fail
  the build loudly if not.
- **[Render cost per request]** → satori+resvg is tens of ms; mitigated by HTTP
  caching and the rarity of crawler refetches. LRU/nginx are documented seams if
  it ever shows up under load.
- **[logo.dev latency on render]** → bounded by a short fetch timeout; any failure
  degrades to the monogram, so a slow logo.dev never stalls the response.
- **[No unit runner → degradation logic untested by unit tests]** → mitigated by
  keeping `card.ts` pure and splitting a framework-free `renderCardPng` core that a
  dependency-free Node smoke script (`web/scripts/og-smoke.mjs`) drives over fixture
  jobs (full / no-salary / no-logo / long-title), plus visual verification of the
  degraded states against a running stack. No Playwright/vitest harness exists in the
  repo, and adding one is out of scope (project convention).

## Migration Plan

Additive, web-tier only — no backend/DB/API change, no data migration.
Deploy the `web/` build (which now bundles the fonts and native resvg binary).
Rollback = redeploy the previous web build; the endpoint and the `og:image` tag
simply disappear, leaving the prior `summary` card. Confirm `@resvg/resvg-js`
loads in the built container before promoting.

## Open Questions

None outstanding — the logo strategy (logo.dev + monogram), the verification
approach (e2e-driven, no unit runner), and the card direction (A) are all
decided.
