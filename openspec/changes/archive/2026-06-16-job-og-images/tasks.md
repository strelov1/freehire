## 1. Dependencies and fonts

- [x] 1.1 Add `satori`, `satori-html`, `@resvg/resvg-js` to `web/package.json` and install
- [x] 1.2 Add the Inter font files (`Inter-Regular.ttf`, `Inter-SemiBold.ttf`, `Inter-Bold.ttf`) under `web/src/lib/server/og/fonts/`
- [x] 1.3 Verify `@resvg/resvg-js` resolves its native binary in the web `Dockerfile` (Node/Alpine musl); adjust the build if the prebuilt is missing

## 2. Render contract (RED first)

- [x] 2.1 Write a failing dependency-free Node smoke test `web/scripts/og-smoke.mjs`: builds fixture `Job`s (full / no-salary / no-logo / long-title), reads the bundled Inter TTFs via `fs`, calls the framework-free render core, and asserts each output begins with the PNG signature and is non-trivially sized. Fails initially because the render core does not exist yet
- [x] 2.2 Add the route `web/src/routes/jobs/[slug]/og.png/+server.ts` that resolves the job via `serverApi(fetch).getJob(slug)` (404 → SvelteKit 404), calls the renderer, and responds `image/png` with `Cache-Control: public, max-age=3600, stale-while-revalidate=86400`

## 3. Rendering module

- [x] 3.1 `web/src/lib/server/og/card.ts` — pure `buildCard(job, { logo }) => string`: direction-A layout (company row, auto-sized clamped title, fixed-order facet chips work_mode→seniority→salary→skills, footer), reusing `formatSalary` from `$lib/enrichment`
- [x] 3.2 Implement the degradation rules in `card.ts`: omit a chip for any missing facet (no null/placeholder), monogram derivation from company name, title font-shrink + 3-line clamp + ellipsis, skills overflow → `+N`
- [x] 3.3 `web/src/lib/server/og/logo.ts` — `resolveLogo(company) => Promise<string|null>`: server-side logo.dev fetch (shared publishable token extracted from `CompanyLogo.svelte`) with a timeout, `200` → `data:` URI, any failure → `null`
- [x] 3.4 `web/src/lib/server/og/render.ts` — split: a framework-free `renderCardPng(job, { fonts, logo }) => Promise<Uint8Array>` (`buildCard` → `satori-html` → `satori` → `@resvg/resvg-js`) that the smoke test from 2.1 drives green, plus a thin SvelteKit wrapper `renderOgPng(job)` that loads the Inter fonts via `read()` from `$app/server`, resolves the logo, and calls the core; wire the wrapper into the endpoint

## 4. Metadata wiring

- [x] 4.1 `web/src/lib/components/Seo.svelte` — add optional `image?: string`: when set emit `og:image` (+ `og:image:width` 1200, `og:image:height` 630, `og:image:type`, `og:image:alt`) and switch `twitter:card` to `summary_large_image` with `twitter:image`; when absent, behaviour unchanged
- [x] 4.2 `web/src/routes/jobs/[slug]/+page.svelte` — pass `image={`${origin}/jobs/${data.job.public_slug}/og.png`}` to `<Seo>`
- [x] 4.3 Verify against a running dev server (`curl /jobs/:slug`) that the job-detail `<head>` contains an absolute `og:image` ending `/og.png`, `og:image:width`/`height`, and `twitter:card` = `summary_large_image`; a list page keeps `summary` with no `og:image`

## 5. Verification

- [x] 5.1 Visually verify the degraded states against a running stack: full job, no-salary, no-logo (monogram), almost-empty, long-title — each looks intentional
- [x] 5.2 `svelte-check` clean and lint not regressed versus the known baseline; `node web/scripts/og-smoke.mjs` green
