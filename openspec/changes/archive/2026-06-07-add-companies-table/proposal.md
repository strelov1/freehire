## Why

Company is currently a free-text field on `jobs`, so there is no way to show a
list of companies or a single company with its jobs. We want companies as a
first-class entity — without paying for joins on the hot read paths.

## What Changes

- Add a `companies` table keyed by a natural `slug` (normalized company name);
  no surrogate `id`.
- Add `jobs.company_slug` — the normalized link key — alongside the existing
  `jobs.company` (kept as the display name). Index it for company-scoped reads.
- The write path (`UpsertJob`) derives `slug` from the company name and upserts
  the company row in the same unit of work, so the table stays populated.
- New read endpoints: `GET /api/v1/companies` (list) and
  `GET /api/v1/companies/:slug` (company + its jobs). Both are join-free: company
  metadata comes from `companies`, its jobs from a single-table filter on
  `jobs.company_slug`. The list's job counts are computed on the fly for now.

Deliberately deferred as seams (not built until needed): denormalized
`job_count` counter, company metadata (logo/website), and stripping legal
suffixes during slug normalization.

## Capabilities

### New Capabilities

- `companies`: storing companies as a slug-keyed entity, linking jobs to a
  company via a denormalized key, and serving company list + company-detail
  reads without joining `jobs`.

### Modified Capabilities
<!-- No existing specs yet; job behavior change is captured under the new `companies` capability. -->

## Impact

- **Schema**: new `migrations/0002_*.sql` (companies table, `jobs.company_slug`,
  index). First schema change after init — applies only on a fresh volume
  (`docker compose down -v && make up`); no versioned migration runner yet.
- **DB layer**: new `internal/db/queries/companies.sql`; `jobs.sql` gains
  `company_slug` and a company-scoped jobs query. Requires `make sqlc`.
- **Pipeline**: `UpsertJob` write path now also normalizes the name and upserts
  the company.
- **API**: new `/api/v1/companies` routes wired in `handlers.Register`.
