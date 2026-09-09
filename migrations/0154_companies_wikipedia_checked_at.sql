-- companies.company_info_wikipedia_checked_at: when the Wikipedia/Wikidata company-info
-- backfill last resolved this company, whether it found a confident match or rejected one.
--
-- Unlike the two existing company-info sources (the YC directory, an external company-info
-- dump), which each process a closed, one-off dataset exactly once, this backfill's target
-- population — companies discovered purely through ATS crawling with no curated-dataset
-- match — grows continuously as new boards are crawled. It therefore needs to run
-- repeatedly without re-querying Wikidata for a company it already resolved, match or not.
--
-- NULL means "never checked by this backfill". A company matched successfully also gets
-- tagline/company_info/company_info_at set in the same write; a rejected or unmatched
-- company gets only this column set, so it is skipped on every later run without needing a
-- second signal to distinguish "checked, no match" from "never checked".
--
-- A nullable ADD COLUMN with no default is catalogue-only in PostgreSQL 11+, so this does not
-- rewrite the table and needs no CONCURRENTLY split.
ALTER TABLE public.companies
    ADD COLUMN IF NOT EXISTS company_info_wikipedia_checked_at timestamptz;
