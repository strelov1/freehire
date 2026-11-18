-- Industries use the same deduplicated, rate-limited request queue as skills and
-- specializations. Keep the original migration immutable for existing installs.
-- Migration 0188's unnamed inline kind check is named dictionary_gaps_kind_check by PostgreSQL.
ALTER TABLE dictionary_gaps
    ADD CONSTRAINT dictionary_gaps_kind_check_v2
    CHECK (kind IN ('specialization', 'skill', 'industry')) NOT VALID;

ALTER TABLE dictionary_gaps
    DROP CONSTRAINT dictionary_gaps_kind_check;

ALTER TABLE dictionary_gaps
    RENAME CONSTRAINT dictionary_gaps_kind_check_v2 TO dictionary_gaps_kind_check;
