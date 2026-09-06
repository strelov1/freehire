## 1. Dictionary: expose the domain→industry mapping as parallel arrays

- [x] 1.1 Add `DomainIndustryPairs() (domains, industries []string)` to
      `internal/dict/industrytag/domains.go`, built from the existing
      `domainIndustry` map, sorted by domain, same length, `pairs[i]` is one
      mapping — the form `RefreshCompanyFacets` will pass as two `text[]`
      parameters.
- [x] 1.2 Unit test: `DomainIndustryPairs` returns one entry per `domainIndustry`
      key, sorted, and stays in sync if the map gains or loses an entry (walk the
      map and assert against the returned pairs rather than hardcoding the list).

## 2. Schema: `companies.industries_derived`

- [x] 2.1 Add migration `migrations/0142_companies_industries_derived.sql`:
      `ALTER TABLE companies ADD COLUMN industries_derived text[] NOT NULL DEFAULT
      '{}'`, plus `migrations/0143_companies_industries_derived_idx.sql`
      (`no-transaction`, `CREATE INDEX CONCURRENTLY ... USING GIN
      (industries_derived)`, split out because CONCURRENTLY must be the only
      statement in its file). `pnpm check:sql` passes on both.
- [x] 2.2 Run `make sqlc` after later steps touch `companies.sql` (placeholder —
      actual regen happens once the query changes in section 3).

## 3. RefreshCompanyFacets: compute industries_derived

- [x] 3.1 In `internal/platform/db/queries/companies.sql`, add CTEs
      (`derived_eligible`, `domain_industry_map`, `derived_ind`) that, per
      company, use the existing `dom` CTE and join the two new
      `mapping_domains`/`mapping_industries` query params (zipped back into rows
      via `WITH ORDINALITY`, since sqlc's analyzer does not resolve the
      two-array form of `unnest(text[], text[])` the way a live Postgres does)
      against `dom.arr`, producing `array_agg(DISTINCT industry)`.
- [x] 3.2 Gate the result to `'{}'` when `cardinality(co.industries) > 0` OR
      `cardinality(COALESCE(dom.arr, '{}')) > 2`, otherwise the mapped set from
      3.1.
- [x] 3.3 Add `industries_derived` to the `SET` clause and to the `IS DISTINCT
      FROM` guard alongside the other facet arrays.
- [x] 3.4 In `cmd/recount-companies`, pass `industrytag.DomainIndustryPairs()` as
      the two new query parameters.
- [x] 3.5 Run `make sqlc` to regenerate `internal/platform/db`.
- [x] 3.6 Integration test (`internal/platform/db`, `-tags=integration`) covering
      the three `industries_derived` scenarios from `specs/companies/spec.md`:
      curated company (empty regardless of domains), ≤2 domains (mapped), >2
      domains (empty). `TestRefreshCompanyFacetsIndustriesDerived` in
      `company_facets_integration_test.go`, passing against real Postgres.

## 4. Simplify the Postgres filter path

- [x] 4.1 In `internal/platform/db/queries/companies.sql` (`ListCompanies`/
      `CountCompanies`), replace the `cardinality(industries) = 0` /
      derived-domain-join predicate for the `industries` param with a plain OR
      against `industries_derived` (same shape as any other array-facet
      predicate); `make sqlc` regenerated.
- [x] 4.2 Removed the now-dead `IndustryDomains`/`industry_domains` param
      plumbing end to end: `internal/api/handler/companies.go` no longer computes
      or passes it (and no longer imports `industrytag`), and the sqlc query no
      longer declares the param.

## 5. Simplify the Meilisearch filter path

- [x] 5.1 In `internal/search/search/company.go`, `CompanyFilterFromValues`:
      replaced the `industries` case's `And(IsUnset("industries"), Eq("domains",
      domain))` loop (via `industrytag.DomainsForIndustries`) with
      `Eq("industries_derived", val)` OR-ed into the same group as
      `Eq("industries", val)`; `industrytag` import dropped, now unused here.
- [x] 5.2 Kept the industries case special in `CompanyFilterFromValues` (as
      before, for the two-source OR) rather than folding it into the
      table-driven `companyFacets` loop — closest to the existing pattern.

## 6. Meilisearch index: new filterable attribute

- [x] 6.1 Added `IndustriesDerived []string` to `CompanyDocument` and its
      `FromCompany` projection in `internal/search/search/company.go`.
- [x] 6.2 Added `industries_derived` to `companySettings()`'s
      `FilterableAttributes`.
- [x] 6.2a Updated the tests the filter-shape change orphaned:
      `company_industries_test.go` (unit, rewritten for the plain-OR shape, no
      more `IsUnset`/domain-translation assertions) and
      `company_integration_test.go`'s `TestIntegration_CompanySearch_IndustryPrecedence`
      (now fixes `IndustriesDerived` directly instead of deriving it from
      `Domains` at query time, plus a fixture above the domain-count threshold).
      Both pass (`go test ./internal/search/search/...` and
      `-tags=integration ... -run TestIntegration_CompanySearch_IndustryPrecedence`
      against real Meilisearch via Docker).
- [ ] 6.3 Deploy this step **before** the schema migration and backfill land, per
      design.md's Migration Plan — the index must carry the attribute before
      `reindex-companies` is run against it.

## 7. Tests: fixtures and backend agreement

- [x] 7.1 Extended `companies_industry_domain_integration_test.go` fixtures:
      `focused-uncurated` (exactly two domains, no curated industry — reachable)
      and `wide-uncurated` (three domains, no curated industry — unreachable).
      `companyFixture` gained `industriesDerived`, seeded into the new column
      directly (a nil fixture value maps to `'{}'`, since the column is NOT
      NULL) rather than exercising `RefreshCompanyFacets` itself (that's
      `internal/platform/db`'s job, see 3.6).
- [x] 7.2 Extended the "the Meilisearch path matches the same companies"
      subtest's industry list with the new fixtures' industries
      (`climate-tech`, `crypto`, `ecommerce`, `gambling`, `hr-tech`);
      `slugsMatchingMeiliFilter`'s `attrs` map gained `industries_derived`, and
      `evalFragment`/`evalUnset` simplified to the new plain-`attr = "value"`
      shape (the parenthesised-conjunction case they used to parse no longer
      exists, since precedence is baked into the materialized column).
- [x] 7.3 `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...`
      clean; `go test ./...` clean. Integration suite run against real
      Postgres/Meilisearch via Docker: `internal/platform/db`
      (`TestRefreshCompanyFacets*`) and `internal/api/handler`
      (`TestListCompaniesIndustryFacetReadsBothColumns`) and
      `internal/search/search` (`TestIntegration_CompanySearch_IndustryPrecedence`)
      all pass. Full `-tags=integration` suite across every package not run in
      this session (would also exercise unrelated pre-existing suites); the
      packages this change touches are covered above.

## 8. Rollout

- [ ] 8.1 Deploy Meilisearch attribute change (section 6) first.
- [ ] 8.2 Run the migration (`migrate`), deploy the code from sections 1-5.
- [ ] 8.3 Run `recount-companies` to backfill `industries_derived`.
- [ ] 8.4 Run `reindex-companies` on its own — never stacked with `make reindex`.
- [ ] 8.5 Spot-check `freenow` (or another known wide-domain, no-curated-industry
      company from the issue) on `/api/v1/companies?industries=` for each of its
      domains' mapped industries — confirm it no longer matches any, since it
      carries more than two domains.
