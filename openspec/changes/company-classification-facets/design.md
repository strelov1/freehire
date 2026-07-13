## Context

`company_type` is derived per-job by the enrichment LLM and aggregated into
`companies.company_types`. A prod spike proved it ill-posed on the ambiguous
business-model middle (product↔startup↔inhouse↔outsource) while reliable only on
the maturity axis — and that axis is derivable from company signals we already
store. `RefreshCompanyFacets` (`internal/db/queries/companies.sql`, a single
set-based SQL pass run by `cmd/recount-companies` every 6h) already recomputes
every company's denormalized facets under an `IS DISTINCT FROM` change-guard — the
natural home for one more computed column.

A task-1.1 prod measurement narrowed the reliable signals:
- **Government source** is clean only for the exclusively-government sources
  `usajobs` and `neogov` (generic ATS like workday/icims/greenhouse carry
  government jobs too, so they are NOT a government signal).
- **business_model / services** has NO clean signal: the `industries` "services"
  keyword is ~87% false-positive (matches product companies), and no source
  cleanly marks an agency — so `business_model` is **out of scope** for this
  change (deferred to a future company-website/registry classifier).

## Goals / Non-Goals

**Goals:**
- One well-posed, deterministic company facet (`maturity`), computed once per
  company from existing signals, honest `NULL` on unknown.
- Zero LLM, zero per-job cost, no new worker — reuse `RefreshCompanyFacets` and its
  timer/guard.
- Additive: `company_type` and everything reading it stay untouched.

**Non-Goals:**
- `business_model` — no reliable deterministic signal exists yet (deferred).
- Removing or deprecating `company_type` (a later change).
- Per-job exposure / Meilisearch faceting (company-level Postgres filter only; a
  `jobview` join can come later if a per-job filter is wanted).

## Decisions

- **Scalar column, membership filter.** Unlike the existing array facets (filtered
  by `&&` overlap), `maturity` is a single-valued `text` column filtered by
  `maturity = ANY($1::text[])` (OR within facet). `NULL` matches nothing — the
  honest-unknown company is simply not returned for this facet. This is a
  deliberate divergence from the array-facet machinery, wired explicitly in
  `ListCompanies`/`CountCompanies`.
- **Pure-SQL rule in `RefreshCompanyFacets`.** Add `source` to the `oj` CTE, add a
  `gov_sig` aggregate (`bool_or(source = ANY(ARRAY['usajobs','neogov']))`) per
  company, then one `CASE` over `gov_sig` + the `companies` columns. Add it to the
  `SET` and to the `IS DISTINCT FROM` guard so the change-guard still
  short-circuits.
- **Rule precedence (maturity):** `government` → `startup` → `enterprise` →
  `scaleup` → `NULL`. Government wins over size (a large gov agency is still
  `government`). Rules:
  - `government` ← `gov_sig` OR `organization_type = 'Government'`
  - `startup` ← `yc_status` non-empty OR (`year_founded >= extract(year from now()) - 7` AND `employee_count <= 50`)
  - `enterprise` ← `employee_count >= 1000`
  - `scaleup` ← `employee_count BETWEEN 51 AND 999`
  - else `NULL`
- **Backfill = recompute.** No data migration beyond the column; one
  `cmd/recount-companies` run rematerializes the whole table. Migration applied to
  prod manually before deploy (per the migrations gotcha).

## Risks / Trade-offs

- **Coverage is partial by design.** Companies with no signal (no gov source, no
  YC, no `employee_count`) are `NULL` — intended honest behavior, not a bug.
- **`current year` in SQL.** The `startup` recency rule uses
  `extract(year from now())`; the recompute is periodic, so the once-a-year
  boundary shift is acceptable and self-heals on the next run.
- **Sqlc scalar-nullable mapping.** The new column is nullable `text`; expose it as
  nullable (omitted/`null` when unknown), and treat an empty filter request as "no
  constraint".
