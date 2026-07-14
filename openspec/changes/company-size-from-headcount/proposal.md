## Why

The company-size facet (`companies.company_sizes`) is aggregated from the LLM's
per-job `enrichment.company_size` guess. A prod spike showed the company's own
stored `employee_count` (from YC-directory / company-info enrichment) is a **more
accurate** size signal than the LLM guess — the LLM infers headcount from a single
posting, while `employee_count` is a recorded fact — and it covers ~34% of the
catalogue. We already store `employee_count`; we're just not using it for the size
facet.

## What Changes

- Make `companies.company_sizes` a **dict-then-LLM hybrid** (the same shape as the
  geography facets): when the company has an `employee_count`, the facet is the
  single authoritative size bucket derived from it; when it does not, the facet
  falls back to the existing distinct union of `enrichment.company_size` over the
  company's open jobs (unchanged behavior).
- The bucketing follows the existing `company_size` vocabulary
  (`1-10` … `1000+`). This is computed in the existing `RefreshCompanyFacets`
  recompute (pure SQL `CASE`), under the same `IS DISTINCT FROM` change-guard.
- **No new column, filter, API field, or frontend change** — the `company_sizes`
  facet, its `company_size` filter param, and the FilterModal pill are all
  unchanged; only the *source* of the facet's values becomes more accurate where
  `employee_count` is known.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `companies`: the "Companies carry derived facet arrays" requirement changes the
  derivation of `company_sizes` from a pure enrichment union to an
  `employee_count`-authoritative bucket with the enrichment union as fallback.

## Impact

- **DB layer:** `internal/db/queries/companies.sql` — in `RefreshCompanyFacets`,
  derive `company_sizes` from `employee_count` when present (bucketed), else the
  current `csize` union. Regenerate with `make sqlc` (query-string change only; no
  schema/model change).
- **No migration, no new field, no handler/frontend/reindex change.**
- **Backfill:** one `cmd/recount-companies` run after deploy rematerializes every
  company's `company_sizes` from the more accurate source.
- **Observable effect:** companies with a known `employee_count` get a single,
  accurate size bucket (replacing a possibly-noisy multi-value LLM aggregation);
  companies without one are unchanged.
