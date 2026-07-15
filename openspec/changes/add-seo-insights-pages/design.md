## Context

`market-insights` shipped four public JSON endpoints over precomputed `insights_*`
rollups. The web app is SvelteKit SSR (adapter-node, `API_INTERNAL_URL`) and already
runs programmatic landing pages (`/collections/[slug]`) and a keyset sitemap index
(`sitemap.xml` → `sitemap-{jobs,companies,pages}.xml`). This change adds a new family
of landing pages that consume the insights data, plus the small API reads they need.

Two current-API gaps block a single-call-per-page design: `GET /insights/roles` has
no category filter (it returns all category×seniority pairs), and `GET
/insights/salary` with an empty seniority returns only the category-aggregate band,
not per-seniority bands. Both rows already exist in the rollups — only the read
queries/params are missing.

## Goals / Non-Goals

**Goals:**
- Hub + per-category salary/skills/roles pages under `/insights`, server-rendered.
- Substantive, indexable pages (auto-intro, tables, internal links, JSON-LD) gated on
  real data so no thin pages ship.
- Reuse existing SSR, collections-landing, and sitemap-index patterns.
- Two minimal API additions, served from the existing rollups (no schema change).

**Non-Goals:**
- Geo/country page matrix, editorial prose/FAQ, charts, velocity pages (later).
- Any rollup/worker/migration change — the rows already exist.
- New auth or non-public surface.

## Decisions

### D1: SSR-live with CDN caching, not prerender

`server-load` fetches the internal API per request and sets `Cache-Control:
s-maxage=3600`. Matches how `/jobs` and `/collections` already render, keeps data
fresh within the rollup's own cadence, and lets nginx/CDN absorb crawler bursts.
Prerender rejected: the covered-category set changes with data, and static pages go
stale between deploys.

### D2: Extend the API rather than read the DB from web

The pages need category-scoped roles and per-category salary. Add these as insights
API reads (roles gains an optional `category`; a category-salary read returns all
seniorities). Rejected: giving the web SSR layer direct Postgres access — it has none
today, and the API boundary is the project's contract. The additions are pure read
queries over the existing rollups + handler params (sqlc regen, no migration).

### D3: Data-driven quality gate derives the covered set

A category is "covered" iff its insights data clears a threshold (min open jobs
and/or a salary band at/above the sample floor). The covered set is computed from the
API, not hard-coded, and is the single source of truth for: hub links, valid page
params (uncovered → 404), and sitemap entries. This keeps thin pages from ever
shipping and self-heals as data changes.

### D4: One shared page component, three thin route wrappers

`salary/[category]`, `skills/[category]`, `roles/[category]` share a common insights
page layout (breadcrumb, auto-intro, data section, internal-link rail, JSON-LD); each
route's `+page.server.ts` fetches its slice and passes a typed view model. Keeps the
SEO scaffolding in one place and the routes thin.

### D5: Auto-intro is templated from data, not LLM

The intro sentence is a deterministic template filled from the fetched aggregates
("Senior backend roles pay a median of $X across N postings"). No LLM call — cheap,
stable, no per-page cost, and it can't hallucinate.

## Risks / Trade-offs

- **Thin content / Google non-indexing** → Mitigation: the D3 gate + Standard content
  (intro, table, internal links, JSON-LD) keep each page substantive; gated-out
  categories 404 rather than ship empty.
- **Crawler load on the API** → Mitigation: `s-maxage=3600` + CDN/nginx cache; the API
  reads are indexed single-row-set lookups on small rollup tables.
- **Cannibalization with `/collections` and `/jobs`** → Mitigation: insights pages
  target aggregate/informational intent ("what pays / what's in demand"), link OUT to
  the transactional `/jobs` filtered views, and use distinct canonical URLs.
- **Category slug vs enrichment value** → the category vocabulary is lowercase tokens
  (e.g. `backend`); page slugs use them directly, validated against the vocab.

## Migration Plan

Pure additive: new web routes + two API reads. Deploy is the standard `release.sh`
(builds api + web). No migration, no worker change. Rollback = revert the routes/reads
(pages disappear; API additions are inert if unused). The sitemap shard only lists
covered pages, so search engines pick them up after deploy.

## Open Questions

- Gate thresholds (min open jobs; salary sample floor for "covered") — pick
  conservative defaults during implementation, tune from Search Console later.
- Category-salary read shape: a new `category` mode on the existing salary endpoint
  vs a dedicated route — decide at implementation, favor the smallest change that
  returns per-seniority bands in one call.
