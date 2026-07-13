## Why

The single `company_type` enrichment field is **ill-posed**. A read-only prod
spike proved it: the enrichment LLM is ~100% correct on distinctive classes
(`government`: 7022/7022 on usajobs, 657/659 on neogov) but the
`product`↔`startup`↔`inhouse`↔`outsource` middle is inherently ambiguous — a YC
company is *both* product *and* startup, so a forced single label has no ground
truth (the LLM labels YC companies 77% `product`, 22% `startup`). A distilled
CPU classifier confirmed the ceiling: it learned the distinctive classes but
collapsed on the middle (macro-F1 0.42, below the trivial baseline). The field
conflates two orthogonal axes (maturity × business-model) and pays LLM tokens
per job to guess a company fact from a single posting.

The **maturity** axis of that field is exactly the part derivable
**deterministically from company-level signals we already store** — no LLM, no
per-job cost, computed once per company. (The business-model axis is *not*
reliably derivable from current signals — a task-1.1 prod measurement showed the
`industries` "services" keyword is 87% false-positive and no source cleanly marks
an agency — so it is deferred to a future change with a better signal, e.g.
company-website classification.)

## What Changes

- Add one **well-posed, deterministic** company-classification facet on the
  maturity axis the old field conflated:
  - `maturity` ∈ {`government`, `startup`, `scaleup`, `enterprise`} — `NULL` = unknown
- Compute it **once per company** (rematerialized) inside the existing
  `RefreshCompanyFacets` recompute SQL, from signals already in the DB
  (`organization_type`, `yc_status`, `employee_count`, `year_founded`, and a
  government **source** signal aggregated from the company's jobs — the
  exclusively-government sources `usajobs`/`neogov`). Pure SQL `CASE`; no LLM, no
  per-job work, no new worker — reuses the 6h `cmd/recount-companies` timer and the
  `IS DISTINCT FROM` change-guard.
- **Honest abstain:** where signals are silent or conflict, the value is `NULL`
  (unknown) rather than a fabricated label — the fix for the fake precision that
  broke `company_type`.
- Expose it as a repeatable filter facet on `GET /api/v1/companies` (OR within the
  facet, AND across facets, composing with `q`) and on `GetCompany`; add a
  FilterModal pill in the web app.
- **Additive and non-breaking:** the old `company_type` enrichment field, its
  search facet, its frontend filter, and its generated contract are left
  **untouched**. Deprecating `company_type` is a later, separate change.

## Capabilities

### New Capabilities

_None._

### Modified Capabilities

- `companies`: the company-list facet-filter requirement gains one new scalar
  facet (`maturity`); the denormalized-recompute requirement gains one new
  deterministic column, maintained in the same `RefreshCompanyFacets` pass under
  the same change-guard.

## Impact

- **Schema:** migration adds one nullable `text` column (`maturity`) to
  `companies`.
- **DB layer:** `internal/db/queries/companies.sql` — extend
  `RefreshCompanyFacets` (add `source` to the `oj` CTE, add a government
  source-signal aggregate, compute the `maturity` `CASE` column, add to `SET` +
  guard); add `maturity` as a membership filter to `ListCompanies`/`CountCompanies`;
  expose in `GetCompany`. Regenerate with `make sqlc`.
- **API/handlers:** parse the new repeatable `maturity` facet param; include the
  field in the company read shape.
- **Frontend:** `web/src/lib/facets.ts` `COMPANY_FACETS` + a FilterModal pill.
- **Backfill:** one `cmd/recount-companies` run after deploy rematerializes every
  company (migration applied manually before deploy, per the migrations gotcha).
- **No LLM, no per-job cost, no reindex** (company facets are Postgres-filtered,
  not Meilisearch). Old `company_type` behavior unchanged.
