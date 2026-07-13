# Tasks

## 1. Rule inputs pinned (prod measurement — DONE)

- [x] 1.1 Targeted prod measurement complete. Findings recorded in `design.md`:
  government source = `usajobs`/`neogov` only (generic ATS carry gov jobs too, so
  excluded); `business_model`/services has no clean signal (industries keyword ~87%
  false-positive) → deferred, out of scope. `maturity` rules + thresholds pinned.

## 2. Schema — one nullable column

- [x] 2.1 Add a migration adding `maturity text` (nullable) to `companies`. Follow
  the existing migration numbering/style. Note it must be applied to prod manually
  before deploy.

## 3. RefreshCompanyFacets — deterministic maturity (SQL)

- [x] 3.1 **RED** — Add an integration test (`//go:build integration`,
  testcontainers) in `internal/db` seeding companies with distinct signals (a YC
  small company → `startup`; a `usajobs`-sourced company → `government`; an
  `organization_type='Government'` company → `government`; a 1000+ employee company →
  `enterprise`; a 200-employee company → `scaleup`; a signal-less company → `NULL`)
  then calling `RefreshCompanyFacets` and asserting each company's `maturity`.
  Confirm it fails (column unset).
- [x] 3.2 **GREEN** — Extend `RefreshCompanyFacets` in
  `internal/db/queries/companies.sql`: add `source` to the `oj` CTE, add a
  per-company `gov_sig` aggregate, compute the `maturity` `CASE` (precedence per
  `design.md`), add it to the `SET` and the `IS DISTINCT FROM` guard. `make sqlc`.
- [x] 3.3 Extend the existing "rewrites nothing when already current" assertion to
  cover the new `maturity` column (guard short-circuits an unchanged company).

## 4. Filter + read exposure

- [x] 4.1 **RED** — Integration test: `ListCompanies`/`CountCompanies` filtering by
  `maturity` (membership, OR within facet), AND-composing with an existing facet and
  `q`, and excluding `NULL`-`maturity` companies. Fails first.
- [x] 4.2 **GREEN** — Add `maturity` as a `= ANY(sqlc.arg(...)::text[])` membership
  filter (empty array = no constraint) to `ListCompanies` and `CountCompanies`;
  `make sqlc`. Expose `maturity` in `GetCompany` / the company read shape (nullable
  → omitted/null when unknown).
- [x] 4.3 Wire the handler to parse the repeatable `maturity` query param and pass
  it through; include the field in the company JSON. Handler test.

## 5. Frontend facet pill

- [x] 5.1 Add `maturity` to `COMPANY_FACETS` in `web/src/lib/facets.ts` (label +
  values) so it renders as a FilterModal pill on the companies page. Run
  `npm run check` + `lint` locally.

## 6. Verify + backfill

- [x] 6.1 `go build ./... && go vet ./... && go test ./...` and
  `go test -tags=integration ./internal/db/` green.
- [x] 6.2 Document the deploy step: apply the migration manually, deploy, then run
  `cmd/recount-companies` once to rematerialize every company.
