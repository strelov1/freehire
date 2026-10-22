## Context

Approved design: `docs/superpowers/specs/2026-10-21-company-hiring-tab-design.md`.
`cmd/rollup-company` already rebuilds, in one transaction, `insights_company_stats`
(per company, per activity day: added/removed/open), `insights_company_growth`
(open now vs 30 days ago) and `insights_company_response` (reply rate). Only the last
is served, inside `GET /api/v1/companies/:slug` as `response`. `/trends` renders a
global added/removed series with `ActivityBars` and a day/week/month toggle, its window
parsed by `parseActivityQuery`.

## Goals / Non-Goals

**Goals:** a Hiring tab per company with its own URL; reuse every rollup and the
`/trends` chart; one new measured fact (posting lifetime).

**Non-Goals:** views per company; market comparison; Discussion as a third tab (its
pages have their own layout without `CompanyHeader`); adding Hiring URLs to the
sitemap (would double ~240k entries, most of them thin — crawlers reach the tab through
the company page's link; revisit with a threshold once the page has measured value).

## Decisions

- **Tab = route, not client state.** `/companies/<slug>/hiring` is shareable,
  server-rendered and indexable; client-side tabs would hide the content from crawlers
  and from links. Mirrors `/companies/<slug>/discussion`.
- **Reply rate is NOT duplicated into the new endpoint.** The Hiring page loads
  `getCompany(slug, 1, 0)` for the header anyway, and that payload already carries the
  gated `response`. One source, one gate.
- **Breakdowns and salary from the existing `facetCounts` call**, scoped by
  `company_slug`, naming only the facets read, and loaded SERVER-side in parallel with
  the company read so a crawler reads them too. No new backend; the numbers match what
  the Jobs tab's filters show, by construction, and each value links into the Jobs tab
  filtered by it. Labels resolve through the same `FACETS` options as the filter panel.
- **Salary is one currency, one period.** Measured on prod, an unfiltered min/max for
  one employer spanned 60 to 7,000,000 (hourly rupees beside yearly dollars). The page
  picks the most common `salary_currency` among the company's YEARLY postings, asks a
  count narrowed to it and `salary_period=year`, and shows the range only from 3 such postings up.
- **Tabs reuse the design system's `TabStrip`**, which already renders `href` tabs as
  anchors; the `/trends` toggle-and-chart logic moves into a shared `ActivityPanel` used
  by both pages rather than being copied.
- **Dense series in SQL** (`ListCompanyActivity`), the same shape as `ListJobActivity`:
  `generate_series` over the window, `LEFT JOIN insights_company_stats` on its primary
  key, `date_trunc` rollup. The endpoint reuses `parseActivityQuery` and the
  `activityPoint` wire type, so `ActivityBars` takes it unchanged.
- **Lifetime counts employer-side closures only** (`unseen`, `employer_closed`,
  `feed_removed`, `expired`). `board_unreachable`/`feed_empty` measure our crawler,
  `source_misattributed`/`moderated` our own decisions — counting them would publish our
  health as a named employer's hiring speed. 180-day window so the figure describes the
  company now. Gate at 5 closures in the handler, beside the reply-rate gates, absent
  (not zero) below it. A separate table rather than a column on
  `insights_company_growth`, whose `HAVING` drops companies with nothing open — exactly
  the ones whose closure history is most telling.
- **Handler lives on `companiesHandlers`** (route `/companies/:slug/hiring`), reusing
  the alias-redirect path of `GetCompany`; registered before `/companies/:slug` is not
  needed since Fiber matches the extra segment distinctly.

## Risks / Trade-offs

- [Empty table until the first rollup after deploy] → the gate renders "not enough
  data yet"; the timer runs nightly.
- [Lifetime biased short by the 180-day window (long-lived postings closed earlier are
  outside it)] → acceptable: the figure answers "how fast do they close now"; the label
  says "median days open, last 6 months".
- [Thin pages for tiny companies] → `noindex, follow` when nothing is open and the
  series is all zero; not in the sitemap.

## Migration Plan

Migration adds `insights_company_lifetime` (new table, no lock on `jobs`); release.sh
applies it before the binary flips. Rollback: revert the binary; the table is inert.
