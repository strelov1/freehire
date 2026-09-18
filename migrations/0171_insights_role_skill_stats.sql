-- Per-role skill demand: the skill distribution WITHIN one role, where a role is
-- the (category, seniority) pair insights_role_stats (0022) already keys on.
--
-- The gap this fills: insights_role_stats answers "which roles are hiring" and
-- insights_skill_stats answers "which skills are in demand", but the second is
-- scoped by category OR country and never by seniority — so Senior Backend and
-- Junior Backend are indistinguishable in the skill rollup.
--
-- Both tables are rebuilt by cmd/rollup-stats as an atomic delete-and-reinsert,
-- in the same transaction as their siblings, so a reader never sees a partial
-- rebuild.

-- No country column, deliberately. insights_skill_stats' own comment records the
-- rule for skill demand — category and country are not crossed in one row —
-- and crossing a THIRD axis here would multiply 52 categories x 8 seniorities by
-- the country cardinality for a slice nobody has asked for. The endpoint serves a
-- country-scoped open_count beside this country-agnostic distribution and says so.
CREATE TABLE public.insights_role_skill_stats (
    category   text    NOT NULL,
    seniority  text    NOT NULL,
    skill      text    NOT NULL,
    -- squawk-ignore prefer-bigint-over-int -- one role's open postings: bounded by a ~12M-row catalogue and in practice by tens of thousands, four orders of magnitude below int's 2.1B ceiling; and integer matches insights_role_stats.open_count and insights_salary_stats.sample_size, so the generated Go stays int32 across the whole insights family rather than one table alone widening to int64
    open_count integer NOT NULL DEFAULT 0,
    PRIMARY KEY (category, seniority, skill)
);

-- Reads select one role and order by demand to take the top-N; this index serves
-- that ranked read.
CREATE INDEX insights_role_skill_stats_role_count_idx
    ON public.insights_role_skill_stats (category, seniority, open_count DESC);

-- The share's denominator, one row per role — at most 52 categories x 8 seniorities.
--
-- (This comment said "27 x 8 = 216" when the migration was applied, which was wrong: 27 is
-- vocab.TechCategories, and the rollup filters on `category <> ''`, not on that set. The
-- first production run measured 371 roles, already past the stated ceiling. Corrected in
-- place because no statement changes and the runner keys on the version name alone — see
-- schema_migrations, which stores no checksum.)
--
-- It is the count of the role's open postings carrying AT LEAST ONE tagged skill,
-- never the role's whole open count. Measured on production 2026-09-18, 11% of the
-- eligible postings carry no tagged skill at all (392,020 -> 348,060): dividing by
-- the open count would fold our own tagging gap into every published share, and
-- because that gap differs per role it would make two roles' shares incomparable —
-- which is the one comparison the surface exists to support.
--
-- A separate table rather than a column on insights_role_stats, which is keyed by
-- (category, seniority, country): this figure is country-agnostic, so it would be
-- meaningless on every non-'' row there. And rather than a value repeated on every
-- insights_role_skill_stats row, which is one fact written a dozen times.
CREATE TABLE public.insights_role_skill_sample (
    category    text    NOT NULL,
    seniority   text    NOT NULL,
    -- squawk-ignore prefer-bigint-over-int -- same argument as open_count above: a count of one role's skill-bearing open postings, and the same int32 the rest of the insights family serves
    sample_size integer NOT NULL DEFAULT 0,
    PRIMARY KEY (category, seniority)
);
