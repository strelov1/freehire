## ADDED Requirements

### Requirement: Monthly salary rollup

The rollup worker SHALL precompute, for every category and every (currency, period),
the 25th, 50th and 75th percentile of the effective salary and the sample size, for
each of the last 12 calendar months including the current one. A posting SHALL
belong to the month it was published — the earlier of `posted_at` and `created_at` —
whether it is open or closed now, because a closed posting still records what was
offered that month. Only canonical (`duplicate_of IS NULL`), public, categorized
postings SHALL count. A month whose sample is below 30 SHALL NOT be stored. The table
SHALL be rebuilt in full on every run, in the same atomic swap as the other insights
rollups, so its history needs no separate backfill.

#### Scenario: A closed posting keeps its month

- **WHEN** a posting published in March with a salary closed in April
- **THEN** it counts in March's sample

#### Scenario: A late-found posting goes to its publication month

- **WHEN** a posting published in March was first seen by freehire in June
- **THEN** it counts in March, not June

#### Scenario: Thin months are not stored

- **WHEN** a (category, currency, period) month has 29 qualifying postings
- **THEN** no row exists for it

#### Scenario: Currencies and periods never mix

- **WHEN** a category's postings in one month state salaries in USD per year and EUR
  per month
- **THEN** each (currency, period) yields its own row

### Requirement: Salary history endpoint

The system SHALL expose a public, unauthenticated
`GET /api/v1/insights/salary/history?category=<category>` returning that category's
stored months as `{"data": [...]}`, each row carrying `currency`, `period`, `month`
(`YYYY-MM`), `sample_size`, `p25`, `p50` and `p75`, ordered by currency, period and
month. `category` SHALL be required and validated like the sibling insights
endpoints. The response SHALL be aggregate-only.

#### Scenario: History for a category

- **WHEN** a client requests the history for `backend`
- **THEN** the response is `200` and every row belongs to `backend`'s stored months

#### Scenario: Missing or unknown category

- **WHEN** the `category` parameter is absent or not a known category
- **THEN** the response is `400`

### Requirement: Salary history chart

The `/insights/salary/<category>` page SHALL draw the history above its bands table
when the chosen (currency, period) has at least 3 stored months: a line through the
monthly medians and a shaded band from the 25th to the 75th percentile. It SHALL
default to the (currency, period) with the largest total sample and offer the others
that also have at least 3 months. A missing month SHALL be a gap, never interpolated.
The page SHALL state that months follow the publication date, that a month needs 30
postings to be shown, and that the figures describe postings on freehire rather than
the whole market. The same figures SHALL be present as a table in the server-rendered
HTML, so the history reads without JavaScript and to a screen reader.

#### Scenario: Default series

- **WHEN** a category has 6 months in USD per year and 4 in EUR per year, with more
  USD postings
- **THEN** the chart shows USD per year and offers EUR per year

#### Scenario: Too little history

- **WHEN** no (currency, period) has 3 stored months
- **THEN** no chart is drawn and the bands table is unchanged

#### Scenario: A gap stays a gap

- **WHEN** the months stored are January, February, April and May
- **THEN** the line breaks at March rather than joining February to April
