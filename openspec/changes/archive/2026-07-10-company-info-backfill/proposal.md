## Why

Our `companies` rows only carry data derived from our own job postings — noisy per-job LLM
facets and job-location geography — with no authoritative company info (headcount, founding
year, HQ, industry, organization type). An external company info dataset, harvested
out-of-band, provides high-coverage authoritative facts for ~158k companies. A one-time
backfill enriches the companies we already have and imports the rest as a visible reference
directory.

## What Changes

- Add authoritative company-info columns to `companies`: `industries`, `year_founded`,
  `employee_count`, `hq_country`, `organization_type`, `tagline`, a `company_info` JSONB for
  low-fill extras (homepage, funding, stock, parent/subsidiaries, activities), plus
  `is_reference` and `company_info_at`.
- Add a run-once host worker `cmd/backfill-company-info` that streams a local JSONL dataset,
  matches each company by normalized-name slug, updates existing companies, and inserts
  unmatched ones as reference rows (`is_reference = true`, no jobs).
- **BREAKING (invariant):** companies may now exist without any job. `DeleteOrphanCompanies`
  no longer removes `is_reference` rows.
- The new columns are independent of the job-derived facets; `RefreshCompanyFacets` does not
  touch them.
- Source anonymity: nothing in the repo names the dataset's origin — no source adapter, no
  `sources/*.yml`, no provenance string. The loader reads a generic file path.
- LinkedIn/social URLs are out of scope (absent from the dataset; no fabricated guesses).
- Surfacing new company info as search facets and in the frontend is a **separate later
  change** (Phase 2); this change lands the data model, loader, and company-detail exposure.

## Capabilities

### New Capabilities
- `company-info`: authoritative company-info attributes on a company and the
  one-time backfill that imports them from an external dataset, matching by slug and
  inserting unmatched companies as reference rows.

### Modified Capabilities
- `companies`: companies may exist as reference rows with no jobs; the orphan cleanup
  preserves reference rows instead of deleting them.

## Impact

- **Schema:** new migration `0041_company_info.sql` (columns + GIN index on
  `industries`).
- **DB queries:** new `UpsertCompanyInfo`; `DeleteOrphanCompanies` gains an
  `AND NOT is_reference` guard. `make sqlc` regen.
- **New command:** `cmd/backfill-company-info` (+ Dockerfile/ops entry if run in prod).
- **Company detail:** the detail view shape maps the new columns into the response
  (`GetCompany` already `SELECT *`).
- **Catalogue size:** company count grows by up to ~158k reference rows.
