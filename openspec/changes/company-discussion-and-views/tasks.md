## 1. Views rollup and endpoint

- [ ] 1.1 Migration `0191_insights_company_views.sql`; queries `DeleteAllInsightsCompanyViews`, `RebuildInsightsCompanyViews`, `GetCompanyViews`; `make sqlc`; integration tests for the rollup scenarios
- [ ] 1.2 `cmd/rollup-company` rebuilds it in its transaction
- [ ] 1.3 Hiring endpoint returns `views`/`views_prev`; integration test; `api-spec.ts` + regenerated docs

## 2. Web

- [ ] 2.1 Views card on the Hiring tab
- [ ] 2.2 `CompanyTabs` gains Discussion with the thread count; `CompanyHeader` drops its Discussion link
- [ ] 2.3 `loadDiscussionSubject` returns the company; the discussion index renders header + tabs when it resolves, the old layout otherwise
- [ ] 2.4 Check both in a browser

## 3. Finish

- [ ] 3.1 Checks, PR, CI, merge, deploy, verify on prod after the next rollup; archive the change
