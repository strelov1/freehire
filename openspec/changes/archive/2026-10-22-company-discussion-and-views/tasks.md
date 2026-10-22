## 1. Views rollup and endpoint

- [x] 1.1 Migration `0191_insights_company_views.sql`; queries `DeleteAllInsightsCompanyViews`, `RebuildInsightsCompanyViews`, `GetCompanyViews`; `make sqlc`; integration tests for the rollup scenarios
- [x] 1.2 `cmd/rollup-company` rebuilds it in its transaction
- [x] 1.3 Hiring endpoint returns `views`/`views_prev`; integration test; `api-spec.ts` + regenerated docs

## 2. Web

- [x] 2.1 Views card on the Hiring tab
- [x] 2.2 `CompanyTabs` gains Discussion with the thread count; `CompanyHeader` drops its Discussion link
- [x] 2.3 `loadDiscussionSubject` returns the company; the discussion index renders header + tabs when it resolves, the old layout otherwise
- [x] 2.4 Check both in a browser

## 3. Finish

- [x] 3.1 Checks, PR, CI, merge, deploy, verify on prod after the next rollup; archive the change
