# company-hiring-tab Specification

## Purpose
The company page's Hiring tab and the endpoint behind it: how one company hires over
time — its open roles and their 30-day change, its reply rate, how long its postings
stay open, its added-vs-removed activity, who it hires and what it pays — so a job
seeker can judge whether it is worth applying.
## Requirements
### Requirement: Company hiring endpoint
The system SHALL serve `GET /api/v1/companies/:slug/hiring` publicly, returning
`{"data": {...}}` with `open`, `open_prev` and `growth` from `insights_company_growth`
(zeros when the company has no row), a dense `series` of `{period, added, removed}` over
the requested window, and `lifetime` (`closed`, `median_days`) from
`insights_company_lifetime`. The window SHALL be parsed exactly as
`/api/v1/stats/jobs-activity` parses it (`granularity`, `from`, `to`, same defaults and
bounds).

#### Scenario: Dense series
- **WHEN** a company has activity on only two days of a 90-day daily window
- **THEN** `series` has one entry per day of the window, with zeros on the other days

#### Scenario: Coarser granularity rolls up
- **WHEN** the request asks for `granularity=week`
- **THEN** each entry sums that week's added and removed counts

#### Scenario: Invalid window
- **WHEN** `granularity` is not `day`, `week` or `month`
- **THEN** the endpoint answers 400

#### Scenario: Unknown and retired slugs
- **WHEN** the slug names no company
- **THEN** the endpoint answers 404
- **WHEN** the slug is a retired alias of a merged company
- **THEN** the endpoint redirects 301 to the canonical company's hiring endpoint

### Requirement: Posting lifetime is gated by sample size
The endpoint SHALL omit `lifetime` entirely when the company has fewer than 5 qualifying
closed postings, or no row. It SHALL never serve a zero or an estimate in its place.

#### Scenario: Below the gate
- **WHEN** a company has 4 qualifying closed postings
- **THEN** the response has no `lifetime` field

#### Scenario: At the gate
- **WHEN** a company has 5 qualifying closed postings
- **THEN** `lifetime` carries `closed = 5` and the median

### Requirement: Hiring tab on the company page
The company page SHALL show two tabs under the company header: **Jobs** linking to
`/companies/<slug>` and **Hiring** linking to `/companies/<slug>/hiring`, rendered as a
tablist whose tabs are links, the current one `aria-selected`. The Hiring page SHALL be server-rendered and canonical
to itself. It SHALL carry `noindex, follow` when the company has no open postings and
no activity in the default window, since such a page has nothing to read.

#### Scenario: An empty company is not indexed
- **WHEN** a company has zero open postings and an all-zero default series
- **THEN** its Hiring page carries `noindex, follow`

#### Scenario: Navigating between tabs
- **WHEN** a visitor on `/companies/acme` clicks **Hiring**
- **THEN** the browser navigates to `/companies/acme/hiring`, the header stays the same, and **Hiring** is the selected tab

### Requirement: Hiring tab content
The Hiring page SHALL show, in order: open postings now with the 30-day change; the
company's reply rate from the existing company `response` fact; the posting lifetime;
the added-vs-removed chart with a day/week/month toggle; the company's open postings
broken down by category, seniority, country, work mode and top skills; and the salary
range from the facet stats. A section with no data SHALL be omitted, a gated figure
(reply rate, lifetime) SHALL read "not enough data yet" instead of a number, and a
failed hiring request SHALL show a retry rather than failing the page.

#### Scenario: Gated figures
- **WHEN** the company payload has no `response` and the hiring payload has no `lifetime`
- **THEN** both cards read "not enough data yet" and neither shows 0

#### Scenario: No salary data
- **WHEN** none of the company's open postings carry a salary
- **THEN** the salary block is absent

#### Scenario: Hiring request fails
- **WHEN** the hiring endpoint errors during the page load
- **THEN** the page still renders the header and tabs, with a retry in place of the figures and chart

