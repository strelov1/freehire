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
- [x] 6.3 Superseded — see 8.1. There is no independent "deploy the attribute"
      step to sequence: the live index only learns the new filterable
      attribute when `cmd/reindex-companies` actually runs, and the Meili
      fallback-to-Postgres path makes the ordering assumed here unnecessary.

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

- [x] 8.1 Superseded by what shipping this actually showed: the Meilisearch
      attribute is not pushed by a separate deploy step at all — `ensure()` on
      the LIVE `companies` index only runs inside `CompanyRebuild.Prepare()`
      (`cmd/reindex-companies`), so there is no independent "push settings
      first" step to sequence ahead of the code deploy. That is fine because
      `internal/api/handler/companies.go`'s Meili path already falls back to
      Postgres on any Meili error (including "attribute not filterable"), so
      the window between code deploy and the next successful
      `reindex-companies` run degrades to the slower path, not to wrong
      results.
- [x] 8.2 PR #2513 merged to `main` (`75e09acb`) and deployed to production via
      `release.sh` on host2. First attempt hit a `lock timeout` on
      `ALTER TABLE companies ADD COLUMN` — the nightly `freehire-pg-backup`
      `pg_dump` was mid-run and held an `AccessShareLock` on every table for
      over an hour; unrelated to this change. Migration rolled back cleanly (no
      partial state), release aborted without touching the live color, and a
      retry once the backup finished applied `0142`/`0143` successfully.
- [x] 8.3 Ran `recount-companies` by hand on host2 rather than waiting for its
      6h45m timer: `companies updated=39201`, backfilling `industries_derived`
      for the whole catalogue.
- [x] 8.4 Ran on its own scheduled timer (`freehire-reindex-companies.timer`,
      12:30 UTC), after the concurrent `freehire-reindexw` (jobs) rebuild that
      was running at deploy time finished: `indexed=227878`. Confirmed via
      `skip-if-reindexing.sh` never having to skip it (the two rebuilds' timers
      did not collide this cycle).
- [x] 8.5 Spot-checked `free-now` (freenow) against the live production API
      (`freehire.me`), not just a local fixture: `industries=[]`, `domains=
      {fintech,gamedev,mobility,saas,travel}` (5, above the ≤2 threshold) —
      confirmed absent from `?industries=fintech`, `?industries=gaming`,
      `?industries=transportation` and `?industries=travel` on both the
      Meilisearch-served path (default routing) and the Postgres-forced path
      (`sort=rating`), which returned the same top-5 companies for
      `?industries=travel` (totals differed by 1 — ordinary staleness between
      live Postgres and the last periodic Meili rebuild, not a disagreement).
