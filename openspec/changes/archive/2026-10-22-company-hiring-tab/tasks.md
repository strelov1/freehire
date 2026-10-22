## 1. Lifetime rollup

- [x] 1.1 Migration `0189_insights_company_lifetime.sql`: table `(company_slug text PK, closed int NOT NULL, median_days real NOT NULL)`
- [x] 1.2 Queries `DeleteAllInsightsCompanyLifetime`, `RebuildInsightsCompanyLifetime` (allowlisted reasons, 180-day window, canonical rows), `GetCompanyLifetime`; `make sqlc`; integration tests for every scenario in the `company-hiring-signal` delta
- [x] 1.3 `cmd/rollup-company` rebuilds it inside the existing transaction and logs the count

## 2. Hiring endpoint

- [x] 2.1 Queries `ListCompanyActivity` (dense, granularity rollup) and `GetCompanyGrowth`; integration tests for dense zeros and week rollup
- [x] 2.2 Handler `GET /api/v1/companies/:slug/hiring` on `companiesHandlers`: `parseActivityQuery` window, 404, alias 301, growth zeros when no row, lifetime gate (5); integration tests
- [x] 2.3 Document the endpoint in `web/src/lib/docs/api-spec.ts` and regenerate (`gen:api-docs`, `gen:openapi`) — NOT `web/static/openapi.yaml`, the deliberately narrow ChatGPT Actions schema

## 3. Web

- [x] 3.1 `api.companyHiring(slug, granularity)` + types in `web/src/lib/api.ts` / `types.ts`
- [x] 3.2 `CompanyTabs.svelte` (Jobs / Hiring over the design system's `TabStrip`), rendered on the company page under the header
- [x] 3.3 Route `companies/[slug]/hiring`: `+page.server.ts` (company, hiring and facet counts in parallel, each non-company failure → null; salary narrowed to one currency, yearly) and `+page.svelte` (figures, chart with toggle, breakdowns, salary, gated "not enough data yet", retry, SEO + noindex rule); the page logic in `$lib/companyHiring.ts` with unit tests; `/trends` moves onto a shared `ActivityPanel`
- [x] 3.4 Run the app and check the tab on a real company in the browser

## 4. Finish

- [x] 4.1 `gofmt`, `go vet ./...`, `go vet -tags=integration ./...`, `go test ./...`, web check/lint/test, then PR → CI → merge → deploy → verify on prod after the next rollup
