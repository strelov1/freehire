## ADDED Requirements

### Requirement: Role pulse rollup

The rollup worker SHALL precompute, per category, inside the same atomic rebuild as the
other insights rollups, over canonical (`duplicate_of IS NULL`), public (`NOT
is_private`), categorized postings:

- the sum of `job_daily_views.page_uniques` of the category's postings over the 30 days
  ending on the freshest complete (before today, UTC) day the views table holds, and over the 30 days before; never
  `uniques`;
- the posting-days the category's postings were visible on freehire within each of those
  two windows, a posting being visible from `created_at` until `closed_at` (or now);
- the number of postings closed in the last 30 days and in the 30 before for a reason
  that describes the posting (`unseen`, `feed_removed`, `probe_expired`,
  `employer_closed`), and the median days from existence (`LEAST(posted_at,
  created_at)`) to close over each window's closes;
- the number of postings whose existence date falls in the last 30 days. No earlier
  window SHALL be computed for it: the comparison measures freehire's coverage growth
  more than the market.

#### Scenario: Only page views count

- **WHEN** a posting's view row carries `uniques` 40 and `page_uniques` 3
- **THEN** the rollup adds 3 to the category's views

#### Scenario: Exposure counts only time on the site

- **WHEN** a posting was first seen 10 days ago and is still open
- **THEN** it adds about 10 posting-days to the current window and none to the earlier one

#### Scenario: Pipeline closures are not hiring

- **WHEN** a posting closed yesterday with `closed_reason` `board_unreachable`,
  `feed_empty`, `expired`, `board_retired`, `moderated`, `source_misattributed` or `''`
- **THEN** it counts in neither the close count nor the median

#### Scenario: Lifetime starts at existence

- **WHEN** a posting published 60 days ago was first seen 5 days ago and closed today as
  `unseen`
- **THEN** its lifetime in the median is 60 days

#### Scenario: Inflow counts publication, not discovery

- **WHEN** a posting published 45 days ago is first seen 3 days ago
- **THEN** it does not count in the inflow

#### Scenario: Duplicates and private postings are excluded

- **WHEN** a posting is a duplicate or private
- **THEN** none of its views, exposure, closes or inflow reach the rollup

### Requirement: Role pulse endpoint

`GET /api/v1/insights/role-pulse?category=<c>` SHALL return, aggregate-only, the
category's rollup figures (both windows for competition and time to close, the last 30
days for inflow), the last day the views cover, and the
category's open count across every seniority (canonical public postings, from the role ×
country rollup's all-countries bucket). `category` SHALL be required and validated
against the category vocabulary; a missing or unknown value SHALL be `400`. A category
with no rollup row SHALL answer zeros and null medians, not an error.

#### Scenario: Known category

- **WHEN** a client requests `/api/v1/insights/role-pulse?category=backend`
- **THEN** the response is `200` with `data.open_count`, `data.competition`,
  `data.time_to_close` and `data.inflow`; competition and time to close carry the
  current and earlier figures, inflow only the current count

#### Scenario: Missing or unknown category

- **WHEN** the `category` parameter is absent or not in the vocabulary
- **THEN** the response is `400` with an `{"error": ...}` body

#### Scenario: Category without data

- **WHEN** a valid category has no rollup row
- **THEN** the response is `200` with zero counts and null medians

### Requirement: Role pulse on the category roles page

`/insights/roles/<category>` SHALL show competition (views per open posting per month),
time to close (median days), each with its direction against the earlier window, and
inflow (new postings in 30 days) with no direction. A figure below its sample floor SHALL
read "not enough data yet" instead of a number, and a direction SHALL be shown only when
both windows clear the floor. The page intro SHALL state the category's all-seniority open count, not the sum of
its seniority rows.

#### Scenario: Thin category

- **WHEN** a category had fewer than 30 qualifying closes in the last 30 days
- **THEN** the time-to-close tile shows no median

#### Scenario: Inflow has no comparison

- **WHEN** the page renders a category's new postings
- **THEN** it shows the 30-day count and no arrow, percentage or earlier count

#### Scenario: Intro counts every seniority

- **WHEN** a category has 400 open postings of which 100 carry a seniority
- **THEN** the intro says "There are 400 open … roles"
