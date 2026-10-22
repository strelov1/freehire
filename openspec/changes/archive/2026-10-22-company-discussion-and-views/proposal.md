## Why

The company page grew Jobs and Hiring tabs (#3442), while its discussion still lives
behind a link in the header and opens a page that looks like a different site. And the
Hiring tab says nothing about attention: how many people look at the company's postings.

## What Changes

- **Discussion becomes the third tab** (`/companies/<slug>/discussion`): the company
  header and the Jobs · Hiring · Discussion strip render above the thread list, the
  open-thread count moves from the header link onto the tab label, and the header link
  goes. A discussion whose company is gone keeps today's header-less layout. Thread and
  new-thread pages are unchanged.
- **Posting views on the Hiring tab**: a fourth card with views of the company's
  postings over the last 30 days and the change against the 30 days before, from
  `job_daily_views.page_uniques` (never `uniques`).
- New rollup `insights_company_views`, rebuilt by `cmd/rollup-company`; the hiring
  endpoint gains `views` and `views_prev`.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `company-hiring-tab`: Discussion joins the tabs; the endpoint and the tab serve views.
- `company-hiring-signal`: adds the per-company posting-views rollup.

## Impact

- Migration: one new table. `insights.sql` (+ sqlc), `cmd/rollup-company`,
  `internal/api/handler/company_hiring.go`, `api-spec.ts` (+ generated docs).
- Web: `CompanyTabs`, `CompanyHeader`, the company discussion index route,
  `lib/server/discussionSubject.ts`, the Hiring page, types.
