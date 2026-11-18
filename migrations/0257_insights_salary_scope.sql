-- The salary rollups behind /salaries, which picks a role, a grade and a country and
-- compares them over time and as a distribution.
--
-- insights_salary_monthly (0216) was per category only. It gains seniority and country,
-- '' meaning every value, the same scope convention as insights_salary_stats (0022), so
-- the history can be read for any role × grade × country the bands can. Existing rows
-- are category-wide and country-agnostic, which is exactly what '' says, so they stay
-- valid until the next cmd/rollup-stats run rebuilds the table.
--
-- insights_salary_stats gains quantiles: the effective salary at every 5th percentile
-- from 5 to 95 (19 values), drawn as the distribution histogram. '{}' until the next
-- rebuild, and from a binary of the previous release, which does not write it.
--
-- Both tables are small rollups rebuilt whole on every run, so swapping the primary key
-- is instant. The defaults let a binary still running the previous release keep
-- inserting while this migration is applied ahead of it.

ALTER TABLE insights_salary_monthly
    ADD COLUMN seniority text NOT NULL DEFAULT '',
    ADD COLUMN country text NOT NULL DEFAULT '',
    DROP CONSTRAINT insights_salary_monthly_pkey,
    -- squawk-ignore adding-serial-primary-key-field, constraint-missing-not-valid -- a rollup of a few thousand rows rebuilt every run; the index build is instant
    ADD PRIMARY KEY (category, seniority, country, currency, period, month);

ALTER TABLE insights_salary_stats
    -- squawk-ignore prefer-bigint-over-int
    ADD COLUMN quantiles integer[] NOT NULL DEFAULT '{}';
