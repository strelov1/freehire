## Context

The site already has a per-job OG pipeline: `src/lib/server/og/{card,render,
fonts,logo}.ts` renders a job card via satori → resvg → PNG, served by
`GET /jobs/:slug/og.png`; `Seo.svelte` emits `og:image` only when a page passes an
`image` prop, and only the job detail page does. Everything else (homepage,
companies, boards, marketing pages) shares links with no image. The reusable
brand primitives (mark, escaping, monogram, logo/footer markup) currently live
inside `card.ts`.

Full brainstormed design (gitignored, for reference):
`docs/superpowers/specs/2026-07-04-og-brand-and-company-cards-design.md`.

## Goals / Non-Goals

**Goals:**
- Every page gets a branded link preview by default.
- Company pages get a logo-forward, on-brand preview.
- Job/company/brand cards share one brand vocabulary and cannot drift.

**Non-Goals:**
- No dynamic homepage OG endpoint — the brand card is a committed static PNG
  (zero runtime cost; figures baked in, bumped on demand).
- No emoji flags — satori ships no emoji font here; HQ is text via `countryLabel`.
- No Go/backend/DB/API changes.

## Decisions

**Three tiers, each overriding the previous.** Static brand PNG (site default) →
job card (`/jobs/:slug/og.png`, exists) → company card (`/companies/:slug/og.png`,
new). Tier selection is just which `image` a page passes to `<Seo>`; the default
is the brand PNG. Alternative — a dynamic `/og.png` endpoint with live figures —
rejected: the brand card is essentially fixed, so a committed PNG is simpler and
free at request time (the user explicitly chose static).

**Shared primitives in `og/shared.ts`.** Extract `OG_WIDTH/OG_HEIGHT`, `esc`,
`monogram`, `MARK_DATA_URI`, `logoBox(logo, name, size)` (parameterised by size,
proportional radius/font, monogram fallback), `Chip`/`chipMarkup`, and
`brandFooter()` from `card.ts`. `card`/`company`/`brand` all import them.
Alternative — duplicate the base64 mark and chip styles per card — rejected: style
drift and a 3× base64 blob.

**`renderMarkupPng(markup, fonts)` primitive.** `render.ts` currently only knows
how to render a `Job`. Generalise: add `renderMarkupPng(markup: string, fonts)`
(satori → resvg → PNG); `renderCardPng(job, …)` becomes a thin wrapper that builds
the job markup and delegates. The company endpoint calls `renderMarkupPng` with
the company markup. Keeps one render/resvg code path.

**Company open-jobs count.** The company OG endpoint fetches the entity via
`getCompany(slug, 1, 0)` and the count via `searchJobs({company_slug: slug}, 1, 0)
.total` — the same read path the detail page uses — so the number matches the page.
Logo via `resolveLogo(company.name)`, degrading to a monogram. Same cache header
as the job endpoint (`public, max-age=3600, stale-while-revalidate=86400`).

**Brand card generation.** `scripts/gen-og.mjs` mirrors `scripts/og-smoke.mjs`
(Vite SSR loader, no new deps): SSR-load `brand.ts` + the render core, load Inter
fonts, render, write `web/static/og.png`. Stat figures are constants in the
generator (mirroring `HomeView.svelte` fallbacks: `2.9M+ open jobs`,
`188K+ companies`, `50+ ATS platforms`). npm `og:gen`. The PNG is committed.

**Default image in `Seo.svelte`.** `image` stays optional; when unset, default to
`` `${page.url.origin}/og.png` `` (import `page` from `$app/state`, SSR-safe). The
`og:image*`/`twitter:image` tags then always emit and the card is always
`summary_large_image`. Job and company pages pass their own image and override.

## Risks / Trade-offs

- **Baked-in figures drift from the live catalogue** → figures are approximate
  ("+") and only on the brand card (generic pages); re-run `og:gen` and re-commit
  when the catalogue grows materially. Job/company cards stay live.
- **satori is flexbox-only, no CSS grid; multi-child elements need `display:flex`**
  → the existing `card.ts` already obeys this; `company.ts`/`brand.ts` follow it,
  and `og-smoke`/`gen-og` render them for real to catch violations.
- **Committing a binary PNG** → small (≈1200×630 flat PNG), regenerable from
  source; acceptable and standard for a static asset.
