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

### Requirement: Discussion tab
The company page's tab strip SHALL carry three tabs: **Jobs**, **Hiring** and
**Discussion** (`/companies/<slug>/discussion`). The Discussion tab's label SHALL carry
the company's open-thread count when it is non-zero (`Discussion · 3`). The discussion
index page SHALL render the company header and the tab strip above the thread list when
the company resolves under the SAME slug, and the header-less layout it had before when
the company is gone, unreachable, or merged into another (a merged company's discussion
stays at the retired slug, whose threads the canonical company's Discussion tab would not
show). The company header SHALL no longer carry its own Discussion link.

#### Scenario: Discussion of a listed company
- **WHEN** a visitor opens `/companies/acme/discussion` and `acme` resolves
- **THEN** the page shows Acme's header, the three tabs with Discussion selected, and the threads

#### Scenario: Discussion of a merged company
- **WHEN** `/companies/acme-inc/discussion` resolves to the canonical company `acme`
- **THEN** the page keeps the header-less layout naming Acme, without tabs

#### Scenario: Discussion that outlived its company
- **WHEN** the company behind the slug no longer exists
- **THEN** the page shows the threads under the existing "no longer listed" subject header, without tabs

### Requirement: Posting views on the Hiring tab
The hiring endpoint SHALL return `views` and `views_prev` from `insights_company_views`
(zeros when the company has no row). The Hiring tab SHALL show the 30-day views and their
change, labelled so a reader knows one visitor counts once per posting per day.

#### Scenario: Views served
- **WHEN** a company's rollup row has `views = 2114`, `views_prev = 1800`
- **THEN** the endpoint returns both and the card shows 2,114 with +314 against the previous 30 days

#### Scenario: No views
- **WHEN** a company has no row
- **THEN** the endpoint returns `views = 0`, `views_prev = 0`

