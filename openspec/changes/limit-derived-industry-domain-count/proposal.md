## Why

#2082 stopped the loud half of the derived-industry defect: a company an importer
has already classified is now answered from that classification alone, so Uber
stopped matching `?industries=gaming`. It does nothing for the quiet half — a
company with **no** curated industry at all, where the job-derived `companies.domains`
union is the only source consulted.

`domains` is a union over every open job the company holds. For a company with many
postings that union describes its hiring range, not its business: `freenow` carries
`industries = {}` and `domains = {fintech, gamedev, healthcare, mobility, other, saas,
travel}`, so it is reachable under five industries instead of one. On production this
affects 9,989 companies with no curated industry and three or more domains (mean 34
open jobs, 1,467 of them at 50+) — a focused company carries one or two domains, so
domain count above that correlates with posting volume, not with business breadth.

## What Changes

- The derived arm of the `industries` facet applies **only when a company carries at
  most two distinct domains**. Above that, its domains describe hiring range and
  contribute no industry.
- The threshold is not expressible as a request-time filter over existing attributes
  (no domain-count attribute exists in Meilisearch), so it is baked into a new
  materialized column, `companies.industries_derived`, computed by
  `RefreshCompanyFacets` — empty whenever the company has a curated industry
  (unchanged precedence from #2082) or more than two domains, otherwise the domains
  mapped to their industries.
- Because precedence and the threshold are now decided at materialization time, both
  filter paths (Postgres and Meilisearch) simplify: `industries=X` becomes a plain OR
  between `industries` and `industries_derived`, with no runtime "is the curated
  column empty" check.
- `internal/dict/industrytag` gains an exported way to read the domain→industry
  mapping as two parallel arrays, so `RefreshCompanyFacets` can pass it as query
  parameters instead of duplicating the table in SQL — one dictionary, one set-based
  `UPDATE`.
- New GIN-indexed column requires a migration, a backfill (`recount-companies`), and a
  standalone `reindex-companies` run (new filterable attribute) — sequenced so the
  live Meilisearch index settings carry the attribute before any query depends on it,
  per the `new-filterable-attr-reindex-window` hazard already recorded for this index.
- `?domains=X` is unaffected — it still filters the raw `companies.domains` union.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `companies`: the industry facet's derived source is now scoped to companies with at
  most two domains, materialized ahead of query time rather than computed per
  request.

## Impact

- `migrations/` — new migration adding `companies.industries_derived text[] NOT NULL
  DEFAULT '{}'` + GIN index.
- `internal/platform/db/queries/companies.sql` (`RefreshCompanyFacets`) — new CTE
  computing the column; `make sqlc`.
- `internal/dict/industrytag` — new exported accessor for the domain→industry
  mapping as parallel arrays.
- `internal/api/handler/companies.go` — Postgres `industries` predicate simplifies to
  an OR over two columns.
- `internal/search/search/company.go` (`CompanyFilterFromValues`) — the derived
  fragment drops its `IsUnset` conjunct; the companies document projection gains
  `industries_derived` as a new filterable attribute.
- `internal/api/handler/companies_industry_domain_integration_test.go` — fixtures
  extended to cover the domain-count boundary.
- Operationally: one migration, one backfill (`recount-companies`), one standalone
  `reindex-companies` run — same hazards `docs/agents/company-facets.md` and the root
  AGENTS.md already document for this index.

## Related

- Issue: https://github.com/strelov1/freehire/issues/2088
- Prior change: `openspec/changes/archive/2026-08-18-fix-derived-industry-precedence/`
  (fixed the curated-company case; its design.md records this remainder and the
  measurements above)
- `docs/agents/company-facets.md` — the ownership rule between curated and derived
  company facets
