-- Time to first reply, beside the response rate it qualifies.
--
-- Nullable on purpose. A company above the sample gate whose applications were all ignored
-- has a response rate of zero and no median at all — reporting a median of zero days there
-- would read as "answers immediately", the exact inversion of what happened.
--
-- No unanswered column: it is applications - answered, and the served payload computes it.
-- The count matters because the median covers answered applications only; presenting a
-- right-censored median bare tells a candidate that employers reply in six days while most
-- of the sample was never replied to at all.
--
-- Applied to a fresh volume by initdb after 0060; on an existing prod volume run this
-- manually (SET ROLE hire) BEFORE deploying code that reads it. Additive, no backfill —
-- cmd/rollup-company fills it on its next run.

ALTER TABLE insights_company_response
    ADD COLUMN IF NOT EXISTS median_reply_days real;
