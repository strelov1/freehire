-- Structured period dates for experience_employments (see internal/candidate/perioddate
-- and the structured-experience-dates change). Additive: the existing period_start/
-- period_end text columns stay untouched here so cmd/backfill-experience-dates has
-- something to read from — a follow-up migration drops them once the backfill and the
-- code that reads these new columns have both landed.
--
-- Plain nullable integers, not a SQL date: a bare "2024" on a CV is real evidence with a
-- real precision, and a date type would force it to lie about a day (and, for a
-- year-only value, a month) nobody stated. Year is NOT NULL-able at the column level (a
-- period can be entirely absent, in which case both columns are NULL) but month alone is
-- nullable within a present period, meaning "no month stated".
ALTER TABLE public.experience_employments
    -- squawk-ignore prefer-bigint-over-int -- a calendar year, CHECK-bound below to [1900, 2100]; nowhere near int32's range
    ADD COLUMN period_start_year integer,
    -- squawk-ignore prefer-bigint-over-smallint -- a calendar month 1-12, CHECK-bound below; nowhere near int16's range
    ADD COLUMN period_start_month smallint,
    -- squawk-ignore prefer-bigint-over-int -- a calendar year, CHECK-bound below to [1900, 2100]; nowhere near int32's range
    ADD COLUMN period_end_year integer,
    -- squawk-ignore prefer-bigint-over-smallint -- a calendar month 1-12, CHECK-bound below; nowhere near int16's range
    ADD COLUMN period_end_month smallint;

-- NOT VALID on every constraint below, same as 0056/0057/0096/0102/0085: the columns are
-- brand new in this same migration, so every existing row reads NULL for them and passes
-- each "IS NULL OR ..." check trivially — there is nothing to validate retroactively, and
-- NOT VALID still skips ADD CONSTRAINT's own table-scanning lock for the (identical) case
-- of a row already holding real data by the time this runs concurrently with writes.
ALTER TABLE public.experience_employments
    -- Same [1900, 2100] bound internal/candidate/perioddate.Sanitize enforces in Go before
    -- any of these columns is written — a DB-level backstop for a write path that bypasses
    -- it (a raw fix-up, a future direct writer), matching the month check's own role.
    ADD CONSTRAINT experience_employments_period_start_year_check
        CHECK (period_start_year IS NULL OR period_start_year BETWEEN 1900 AND 2100) NOT VALID,
    ADD CONSTRAINT experience_employments_period_end_year_check
        CHECK (period_end_year IS NULL OR period_end_year BETWEEN 1900 AND 2100) NOT VALID,
    ADD CONSTRAINT experience_employments_period_start_month_check
        CHECK (period_start_month IS NULL OR period_start_month BETWEEN 1 AND 12) NOT VALID,
    ADD CONSTRAINT experience_employments_period_end_month_check
        CHECK (period_end_month IS NULL OR period_end_month BETWEEN 1 AND 12) NOT VALID,
    -- A month with no year would have no meaning to sort or display by.
    ADD CONSTRAINT experience_employments_period_start_month_needs_year_check
        CHECK (period_start_month IS NULL OR period_start_year IS NOT NULL) NOT VALID,
    ADD CONSTRAINT experience_employments_period_end_month_needs_year_check
        CHECK (period_end_month IS NULL OR period_end_year IS NOT NULL) NOT VALID;

-- Replaces experience_employments_user_idx (0047): the old index ordered period_start
-- lexicographically, which is wrong whenever a candidate's roles mix a bare year with a
-- month-and-year label — the very bug cmd/backfill-experience-dates and this column pair
-- exist to fix. NULLS LAST keeps an employment with no start date sorting after every
-- dated one, matching period_sort.go's old "unknown sorts last" behavior.
--
-- squawk-ignore require-concurrent-index-deletion -- experience_employments holds per-candidate work-history rows, not a crawl/search hot path table (see 0129, 0137); the momentary lock is not worth a no-transaction file
DROP INDEX IF EXISTS public.experience_employments_user_idx;

-- squawk-ignore require-concurrent-index-creation -- same reasoning as the DROP above.
CREATE INDEX experience_employments_user_idx
    ON public.experience_employments (
        user_id,
        is_current DESC,
        period_start_year DESC NULLS LAST,
        period_start_month DESC NULLS LAST
    );
