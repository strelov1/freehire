## Why

Link previews are weak: the homepage and every non-job page emit no `og:image`
(only `/jobs/:slug` does), so sharing `/`, `/companies/*`, `/b/*`, and the
marketing pages yields a bare text card. The company detail page has no branded
preview even though the company logo is a strong, recognisable visual.

## What Changes

- Add a **static brand OG image** (`web/static/og.png`, 1200×630) with the
  freehire mark, the headline "Open source job aggregator that covers all ATS",
  and a stat-strip (open jobs / companies / ATS platforms). It becomes the
  **default `og:image` for every page** that does not supply its own.
- Add a **per-company OG image endpoint** (`GET /companies/:slug/og.png`) — a
  hero company logo (monogram fallback), name, live open-jobs count, optional
  tagline, and industry/HQ chips. The company detail page references it.
- **Bigger company logo** on the existing per-job OG card (implementation
  polish; no spec-level behaviour change).
- Refactor the shared OG primitives (mark, escaping, monogram, parameterised
  logo block, chips, footer) into one module so the job/company/brand cards share
  one brand vocabulary and cannot drift.

## Capabilities

### New Capabilities

- `og-brand-image`: A committed static 1200×630 brand OG image and the site-wide
  default-`og:image` behaviour that serves it for any page without a
  page-specific preview.
- `company-og-images`: On-demand 1200×630 Open Graph preview for a company,
  served from a dedicated web endpoint and populated from the company's served
  fields plus its live open-jobs count, with logo/monogram and sparse-data
  degradation.

### Modified Capabilities

<!-- None. The per-job logo-size bump and the shared-primitive refactor are
     implementation details; job-og-images requirements are unchanged. -->

## Impact

- **Frontend only** (`web/`, SvelteKit). No Go/backend/DB/API changes.
- New: `src/lib/server/og/{shared,brand,company}.ts`,
  `src/routes/companies/[slug]/og.png/+server.ts`, `scripts/gen-og.mjs`,
  `web/static/og.png` (generated, committed).
- Edited: `src/lib/server/og/{card,render}.ts`,
  `src/lib/components/Seo.svelte`,
  `src/routes/companies/[slug]/+page.svelte`, `web/package.json` (`og:gen`).
- Reuses existing satori/resvg render core, `resolveLogo`, `loadOgFonts`,
  `countryLabel`, and the `og-smoke.mjs` harness pattern.
