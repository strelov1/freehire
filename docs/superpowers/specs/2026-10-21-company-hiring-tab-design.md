# Company hiring tab — design

Date: 2026-10-21

## Goal

A job seeker on a company page should be able to answer one question: **is it worth
applying here?** Today the company page lists postings and nothing about how the
company hires. Add a second tab, **Hiring**, next to the job list, carrying the
company's own trend (the `/trends` chart scoped to one employer) plus the few figures
that bear on that question.

## Shape

- Two tabs under the company header: **Jobs** (`/companies/<slug>`, unchanged) and
  **Hiring** (`/companies/<slug>/hiring`, new). Each tab is its own URL: shareable,
  server-rendered, indexable. A tab is a link, not client state.
- A `CompanyTabs.svelte` component renders the two links with `aria-current="page"` on
  the active one; both pages render `CompanyHeader` + `CompanyTabs`.
- **Discussion stays where it is** (the link in `CompanyHeader`). Its pages have their
  own layout and do not render the header; making it a third tab is a separate change.

## What the Hiring tab shows (top to bottom)

1. **Three figures in a row**
   - **Open now and change over 30 days** — `insights_company_growth.open_count` and
     `open_count - open_count_prev` (the rollup's existing 30-day window).
   - **Reply rate** — the existing `companyResponse` fact (rate, unanswered count,
     median days to first reply), with its existing sample gates. Below the gate the
     card says "not enough data yet", never 0%.
   - **How long a posting stays open** — NEW: median days from `created_at` to
     `closed_at` over the company's postings closed in the last 180 days. Gated: fewer
     than 5 such postings → "not enough data yet".
2. **Activity chart** — added vs removed per day/week/month, reusing `ActivityBars`
   and the `/trends` granularity toggle and window defaults (90 days / 52 weeks /
   24 months).
3. **Who they hire** — category, seniority, country, work mode and top skills of the
   company's OPEN postings, read from the existing `facetCounts` call scoped by
   `company_slug` (no new backend).
4. **Salary** — min/max range from the same call's facet stats. The block is absent
   when the company's postings carry no salary.

A company with no data in a section hides that section; a company with no data at all
shows one line saying so. The page stays 200 either way (it is reached from the Jobs
tab of a company that exists).

## Backend

### New endpoint

`GET /api/v1/companies/:slug/hiring?granularity=day|week|month&from=&to=`, public, same
read limiter as `GET /companies/:slug`. Window parsing reuses `parseActivityQuery`, so
defaults and bounds match `/stats/jobs-activity`. Unknown slug → 404; a retired slug
redirects the same way `GetCompany` does.

```json
{"data": {
  "open": 42, "open_prev": 30, "growth": 12,
  "series": [{"period": "2026-10-01", "added": 3, "removed": 1}],
  "lifetime": {"closed": 37, "median_days": 21.5}
}}
```

`lifetime` is omitted below its gate (absent, not zero), the convention
`companyResponse` already follows. The reply rate is NOT repeated here: the page loads
`getCompany` for the header anyway, and that payload already carries the gated
`response`.

### Series query

`insights_company_stats` holds rows only on a company's activity days. The new
`ListCompanyActivity` query densifies it the way `ListJobActivity` densifies
`job_daily_stats`: `generate_series` over the window, `LEFT JOIN` on
`(company_slug, day)` (primary key), roll up with `date_trunc(unit, …)`. Gap days are
zeros.

### Posting lifetime (new rollup)

New table `insights_company_lifetime (company_slug PK, closed int, median_days real)`,
rebuilt by `cmd/rollup-company` inside its existing transaction (delete + insert, like
the other three tables), so it never diverges from them.

It counts only closures that say something about the EMPLOYER: `closed_reason IN
('unseen', 'employer_closed', 'feed_removed', 'expired')`. `board_unreachable`,
`feed_empty`, `source_misattributed` and `moderated` are our own access problems or
our own decisions; counting them would report our crawler's health as the company's
hiring speed. Same canonical-row filter as the sibling rollups (`company_slug <> ''
AND duplicate_of IS NULL`), closed within the last 180 days.

The sample gate (5) lives in the handler beside `responseSampleGate`/`replySampleGate`,
with the same "judgement, not a measurement" note.

## Frontend

- `web/src/routes/companies/[slug]/hiring/+page.server.ts` — loads the company
  (`getCompany`) and the hiring payload in parallel; a failed hiring call renders the
  page with a retry, like `/trends`.
- `+page.svelte` — header, tabs, figures, chart (same request-sequencing as `/trends`),
  breakdowns, salary. Breakdowns and salary load client-side from `facetCounts` naming
  only the facets they read.
- SEO: title "`<Company>` hiring trends — open roles, reply rate · freehire",
  canonical to itself, breadcrumb `Companies › <Company> › Hiring`, `Dataset` JSON-LD
  pointing at the endpoint (as `/trends` does). `noindex, follow` when nothing is open and the default series is all zero. NOT added
to `sitemap-companies.xml`: that would double ~240k entries, most of them thin;
crawlers reach the tab through the company page's link.

## Out of scope (seams, not built)

- Views of the company's postings (`job_daily_views.page_uniques`).
- Comparison against the market ("replies faster than 70% of companies").
- Discussion as a third tab.

## Testing

- Integration: `ListCompanyActivity` (dense zeros, granularity rollup); the lifetime
  rebuild (allowlisted reasons only, 180-day window, duplicates excluded); the
  endpoint (404, gates omit fields, window validation 400).
- Web: the page renders with every section empty and with the hiring call failing.
- Before pushing: `go vet -tags=integration ./...`, `make sqlc` clean, layering table
  unchanged (no new package).

## Rollout

Migration first (release.sh runs it), then the binary; the table is empty until the
next `cmd/rollup-company` run, which the gate already renders as "not enough data
yet". No Meilisearch settings change.
