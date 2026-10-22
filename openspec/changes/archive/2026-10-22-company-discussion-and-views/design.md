## Context

Follows #3442 (Hiring tab). `loadDiscussionSubject` already fetches the full company for
a company discussion (following a merge redirect) and maps it down to a
`DiscussionSubject`. `job_daily_views` holds ~8M rows per 30 days (measured 2026-10-22),
so a per-request sum joined to `jobs` is not affordable.

## Goals / Non-Goals

**Goals:** Discussion as the third tab on the index page; a views figure on Hiring.
**Non-Goals:** tabs on thread / new-thread pages (deeper than a tab); a views time series.

## Decisions

- **Hand the whole company through `loadDiscussionSubject`** (`company` beside
  `subject`) instead of a second `getCompany` call. The header and tabs use
  `company.slug`, so a merge-redirected discussion shows the canonical company.
- **Thread count moves into `CompanyTabs`** (same client-side `countThreads`, same
  "failure hides it" rule) and the header's Discussion link is removed — two entry
  points to the same page side by side would be noise.
- **Views are a nightly scalar pair**, `insights_company_views (company_slug, views,
  views_prev)`, rebuilt in `cmd/rollup-company`'s transaction. Duplicates are included:
  attention paid to a repost is attention paid to the company. `page_uniques` only, per
  the repository's view-count rule.
- The card's label states the unit: summed per-posting daily visitors, not distinct
  people.

## Risks / Trade-offs

- [The views join scans ~16M rows nightly] → one more pass in a job that already scans
  `jobs` three times; measured after deploy.
- [Views lag by up to a day (`rollup-views` runs nightly)] → the label says "last 30 days".
