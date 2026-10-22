## Why

A job seeker on a company page sees the company's postings and nothing about how it
hires, so "is it worth applying here?" has no answer. The per-company daily hiring
series (`insights_company_stats`) has been rebuilt nightly since July and is read by
nothing; the reply-rate fact is served but buried in the company payload.

## What Changes

- A second tab on the company page, **Hiring** (`/companies/<slug>/hiring`), beside
  the existing job list, which becomes the **Jobs** tab. Each tab is its own URL.
- The Hiring tab shows: open postings now and the 30-day change; the reply rate; how
  long a posting typically stays open; the added-vs-removed activity chart (the
  `/trends` chart for one company); who the company hires (category, seniority,
  country, work mode, skills); and the salary range when postings carry one.
- New public endpoint `GET /api/v1/companies/:slug/hiring` serving the series, the
  open/growth scalar and the posting-lifetime fact.
- New rollup `insights_company_lifetime`: per-company median days from posting to an
  employer-side closure, rebuilt by `cmd/rollup-company` in its existing transaction.

## Capabilities

### New Capabilities
- `company-hiring-tab`: the Hiring tab page and the `/companies/:slug/hiring` endpoint
  that feeds it.

### Modified Capabilities
- `company-hiring-signal`: adds the per-company posting-lifetime rollup beside the
  existing velocity, growth and response rollups.

## Impact

- Migration: one new table (`insights_company_lifetime`).
- `internal/platform/db/queries/insights.sql` (+ regenerated sqlc), `cmd/rollup-company`.
- `internal/api/handler` (new route on the companies handler).
- `web/src/routes/companies/[slug]/hiring/`, a new `CompanyTabs` component, the
  company page, `web/src/lib/api.ts`, `sitemap-companies.xml`, `web/static/openapi.yaml`.
- No Meilisearch settings change; breakdowns reuse the existing facet-count call.
