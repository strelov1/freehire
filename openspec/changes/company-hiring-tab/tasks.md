## 1. Lifetime rollup

- [ ] 1.1 Migration `0182_insights_company_lifetime.sql`: table `(company_slug text PK, closed int NOT NULL, median_days real NOT NULL)`
- [ ] 1.2 Queries `DeleteAllInsightsCompanyLifetime`, `RebuildInsightsCompanyLifetime` (allowlisted reasons, 180-day window, canonical rows), `GetCompanyLifetime`; `make sqlc`; integration tests for every scenario in the `company-hiring-signal` delta
- [ ] 1.3 `cmd/rollup-company` rebuilds it inside the existing transaction and logs the count

## 2. Hiring endpoint

- [ ] 2.1 Queries `ListCompanyActivity` (dense, granularity rollup) and `GetCompanyGrowth`; integration tests for dense zeros and week rollup
- [ ] 2.2 Handler `GET /api/v1/companies/:slug/hiring` on `companiesHandlers`: `parseActivityQuery` window, 404, alias 301, growth zeros when no row, lifetime gate (5); integration tests
- [ ] 2.3 Document the endpoint in `web/static/openapi.yaml`

## 3. Web

- [ ] 3.1 `api.companyHiring(slug, granularity)` + types in `web/src/lib/api.ts` / `types.ts`
- [ ] 3.2 `CompanyTabs.svelte` (Jobs / Hiring, `aria-current`), rendered on the company page under the header
- [ ] 3.3 Route `companies/[slug]/hiring`: `+page.server.ts` (company + hiring in parallel, hiring failure → null) and `+page.svelte` (figures, chart with toggle, breakdowns, salary, gated "not enough data yet", retry, SEO + noindex rule); tests for the empty and failed states
- [ ] 3.4 Run the app and check the tab on a real company in the browser

## 4. Finish

- [ ] 4.1 `gofmt`, `go vet ./...`, `go vet -tags=integration ./...`, `go test ./...`, web check/lint/test, then PR → CI → merge → deploy → verify on prod after the next rollup
