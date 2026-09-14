-- seniority is the mentor's own optional statement of their level, drawn from the
-- platform's one seniority vocabulary (internal/dict/vocab.SeniorityValues) rather than a
-- second, mentor-only list — the same "one vocabulary" rule this repo already holds for
-- company legal-form normalization. Empty means unset, not "no seniority": a mentor who
-- leaves it blank is unaffected by a directory search narrowed by seniority, never
-- excluded from the directory itself.
--
-- NOT NULL DEFAULT '' is additive on Postgres 11+ (a constant default needs no table
-- rewrite), so this needs no backfill step.
ALTER TABLE mentors ADD COLUMN seniority text NOT NULL DEFAULT '';
