## MODIFIED Requirements

### Requirement: Additive rollup and materialized counter update

For each processed file, the worker SHALL apply its per-`(day, slug)` uniques with
an additive upsert into `job_daily_views(day, job_id, uniques, page_uniques)`
(`ON CONFLICT (day, job_id) DO UPDATE SET uniques = uniques + EXCLUDED.uniques,
page_uniques = page_uniques + EXCLUDED.page_uniques`) and
add the combined per-`(day, job)` delta to `jobs.view_count`, in a single batched
statement. Slugs that do not resolve to a job SHALL be ignored. Additivity ensures
a day whose lines span two files sums correctly across both.

`uniques` SHALL carry the count of distinct visitors who produced **either**
counted signal, unchanged in meaning and value from before this change — `GET
/api/v1/stats/catalog` reads it. `page_uniques` SHALL carry the count of distinct
visitors who produced a **page open**, which is bot-filtered, so a consumer that
must not be swayed by crawler traffic can rank on it. Because API reads are not
bot-filtered, `page_uniques` is the only counter of the two that describes people.

The two counters SHALL be deduplicated independently over the same
`(visitor, slug, day)` key, and `page_uniques` SHALL therefore never exceed
`uniques`. They SHALL NOT be made to sum with an API count: a visitor who both
opens the page and reads the API on the same day is one visitor in `uniques` and
one visitor in `page_uniques`. Adding the signal kind to the shared dedup key
would make that visitor count twice in `uniques` and silently restate a published
figure, which is exactly what this change must not do.

`page_uniques` SHALL NOT be backfilled over historical rows. Rows written before
this change hold zero, and the column is correct from the first run after deploy.

#### Scenario: Counter reflects a processed file

- **WHEN** the worker processes a file in which a job received 4 unique views on a day
- **THEN** `job_daily_views` holds a row `(day, job_id, 4, …)`
- **AND** the job's `view_count` is increased by 4

#### Scenario: The two signals are materialized separately

- **WHEN** the worker processes a file in which three distinct visitors opened a
  job's page and seven other distinct visitors read it through the API on a day
- **THEN** the `job_daily_views` row for `(day, job)` holds `uniques = 10` and
  `page_uniques = 3`

#### Scenario: A visitor doing both counts once in each counter

- **WHEN** one visitor both opens a job's page and reads it through the API on the
  same day, and no one else views it
- **THEN** the `job_daily_views` row for `(day, job)` holds `uniques = 1` and
  `page_uniques = 1`

#### Scenario: A bot's page open counts in neither

- **WHEN** a visitor whose User-Agent matches the known-bot list opens a job's page
  and makes no API read
- **THEN** that visitor is counted in neither `uniques` nor `page_uniques`

#### Scenario: A job read only through the API has no page uniques

- **WHEN** a job's only counted lines on a day are `GET /api/v1/jobs/<slug>`
- **THEN** its `page_uniques` for that day is 0
- **AND** its `uniques` for that day is the API count

#### Scenario: A day split across two files sums additively

- **WHEN** the worker processes two files that each contribute uniques for the same
  `(day, job)`
- **THEN** the `job_daily_views` row for `(day, job)` holds the sum of both files'
  contributions in both `uniques` and `page_uniques`

#### Scenario: Unknown slug is ignored

- **WHEN** a counted line references a slug that matches no job
- **THEN** the worker skips it without error and processes the rest
