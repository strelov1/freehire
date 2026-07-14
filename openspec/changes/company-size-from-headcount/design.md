## Context

`RefreshCompanyFacets` (`internal/db/queries/companies.sql`) aggregates
`company_sizes` as the distinct union of `enrichment.company_size` over a
company's open jobs (the `csize` CTE). A prod spike found the company's stored
`employee_count` is a more accurate size signal than the LLM guess (34% coverage,
and where both exist the deterministic bucket is the more trustworthy — a recorded
fact vs a per-posting inference). The recompute already reads company columns and
runs under an `IS DISTINCT FROM` change-guard, so this is a one-CTE change.

## Goals / Non-Goals

**Goals:**
- `company_sizes` = `employee_count` bucket when known, else the current
  enrichment union — a dict-then-LLM hybrid, in place, more accurate.
- Zero new column/field/filter/frontend; reuse the recompute + guard.

**Non-Goals:**
- A separate size field or a new filter (the `company_size` facet/param/pill are
  unchanged).
- Changing the `company_size` *vocabulary* or the per-job `enrichment.company_size`
  (still produced and still the fallback source).

## Decisions

- **Compute the final `company_sizes` in a CTE** (mirroring the `mat` CTE added by
  the maturity change) so both the `SET` and the `IS DISTINCT FROM` guard reference
  one value, no `CASE` duplication:
  ```
  csize_final AS (
    SELECT co.slug AS company_slug,
      CASE WHEN co.employee_count IS NULL THEN COALESCE(cs.arr, '{}')
           WHEN co.employee_count <= 10   THEN ARRAY['1-10']
           WHEN co.employee_count <= 50   THEN ARRAY['11-50']
           WHEN co.employee_count <= 200  THEN ARRAY['51-200']
           WHEN co.employee_count <= 500  THEN ARRAY['201-500']
           WHEN co.employee_count <= 1000 THEN ARRAY['501-1000']
           ELSE ARRAY['1000+'] END AS arr
    FROM companies co LEFT JOIN csize cs ON cs.company_slug = co.slug
  )
  ```
  Then `SET company_sizes = csize_final.arr`, guard
  `c.company_sizes IS DISTINCT FROM csize_final.arr`, and `LEFT JOIN csize_final`.
- **Single-element array.** The `company_sizes` filter is array-overlap (`&&`), so
  a one-bucket array `{201-500}` filters correctly with no filter/API change.
- **Backfill = recompute.** No migration; one `cmd/recount-companies` run
  rematerializes `company_sizes` from the better source.

## Risks / Trade-offs

- **Existing values change.** Companies with an `employee_count` may see their
  `company_sizes` shift from a noisy multi-value LLM aggregation to the single
  authoritative bucket. That is the intended improvement, but it is a visible facet
  change — call it out in the changelog if announced.
- **Bucket boundary vs the LLM's brackets.** The bucket edges match the existing
  `company_size` vocabulary exactly, so a headcount of 200 maps to `51-200` (not
  `201-500`) — consistent with how the LLM brackets are labelled.
- **`employee_count` staleness.** It is only as fresh as the last company-info /
  YC-directory enrichment; an outdated headcount yields an outdated bucket. Same
  trust model as every other use of `employee_count`; acceptable.
