## 1. Shared OG primitives (refactor)

- [x] 1.1 Create `src/lib/server/og/shared.ts` exporting `OG_WIDTH`, `OG_HEIGHT`, `esc`, `monogram`, `MARK_DATA_URI`, `Chip` type, `chipMarkup`, `logoBox(logo, name, size)` (parameterised size, proportional radius/font, monogram fallback), and `brandFooter()` — moved verbatim in behaviour from `card.ts`.
- [x] 1.2 Rewire `card.ts` to import from `shared.ts` (drop its local copies); keep `buildCard` output byte-identical except the logo size handled in task 3.
- [x] 1.3 Confirm `node scripts/og-smoke.mjs` still renders all fixtures to valid PNGs after the refactor.

## 2. Render primitive

- [x] 2.1 Add `renderMarkupPng(markup: string, fonts)` to `render.ts` (satori → resvg → PNG) and make `renderCardPng(job, {fonts, logo})` delegate to it via `html(buildCard(...))`.
- [x] 2.2 Confirm `node scripts/og-smoke.mjs` still passes (job path unchanged).

## 3. Job card — bigger logo

- [x] 3.1 In `card.ts`, render the company logo via `logoBox(logo, job.company, 96)` (72→96px) and use `brandFooter()`; re-run `og:smoke` and visually inspect `/tmp/og-smoke/*.png`.

## 4. Company OG card + endpoint

- [x] 4.1 Create `src/lib/server/og/company.ts` — `buildCompanyCard(company, { logo, openJobs })`: hero `logoBox(…,140)`, name, "N open jobs" (singular/plural + `Intl.NumberFormat` grouping), optional tagline (omitted when empty), `industries[0]` + `countryLabel(hq_country)` chips (omitted when absent), `brandFooter()`. Satori flexbox-only.
- [x] 4.2 Extend the smoke harness (add company fixtures to `og-smoke.mjs` or a sibling) covering full / no-logo / no-tagline-no-chips / zero-jobs cases, and assert each renders a valid PNG (RED → GREEN for the card).
- [x] 4.3 Create `src/routes/companies/[slug]/og.png/+server.ts` mirroring the job endpoint: `getCompany(slug,1,0)` (404→404), `searchJobs({company_slug:slug},1,0).total`, `resolveLogo(company.name)` + `loadOgFonts()`, `renderMarkupPng(html(buildCompanyCard(...)), fonts)`, cache header `public, max-age=3600, stale-while-revalidate=86400`.
- [x] 4.4 In `src/routes/companies/[slug]/+page.svelte`, pass `image={`${origin}/companies/${data.slug}/og.png`}` to `<Seo>`.

## 5. Brand card + static generation

- [x] 5.1 Create `src/lib/server/og/brand.ts` — `buildBrandCard({ stats })`: top-left mark + wordmark, centre headline "Open source job aggregator that covers all ATS", 3-column stat-strip (big value over muted label), `brandFooter()`.
- [x] 5.2 Create `scripts/gen-og.mjs` (mirror `og-smoke.mjs`): SSR-load `brand.ts` + render core, load fonts, render with the stat constants (`2.9M+`/`open jobs`, `188K+`/`companies`, `50+`/`ATS platforms`), write `web/static/og.png`; assert the output is a valid PNG.
- [x] 5.3 Add `"og:gen": "node scripts/gen-og.mjs"` to `web/package.json`; run it and commit `web/static/og.png`.

## 6. Default og:image site-wide

- [x] 6.1 In `src/lib/components/Seo.svelte`, default `image` to `${page.url.origin}/og.png` (import `page` from `$app/state`) when unset, so `og:image`/`twitter:image` always emit as `summary_large_image`; pages passing their own image override.

## 7. Verification

- [x] 7.1 `node scripts/gen-og.mjs` and `node scripts/og-smoke.mjs` — inspect brand + job + company PNGs in `/tmp/og-smoke/` and `web/static/og.png`.
- [x] 7.2 Local SSR (dev, `VITE_API_URL` at prod per the SSR-verify note): fetch `/companies/<slug>/og.png` and `/jobs/<slug>/og.png`, save + eyeball logos; view-source `/` and `/companies/<slug>` to confirm `og:image` (brand default on `/`, per-company on the company page).
- [x] 7.3 `npm run check` and `npm run lint` clean.
