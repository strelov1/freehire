-- migrate: no-transaction
--
-- Adds 'board_name' to company_slug_aliases.reason's allowed values (see 0112): a company whose
-- name was its own ATS board id — "fa-etjg-saasfaprod1.fa.ocs.oraclecloud.com/Maggianos", a
-- Paylocity GUID — renamed by cmd/backfill-company-names to the name the ATS states for that
-- board. The rename re-keys the company's slug, and the old /companies/<slug> page had been
-- served and announced (IndexNow announces every company with open postings), so the old slug
-- must 301 to the new one rather than 404.
--
-- It needs its own label rather than reusing 'spelling' or 'legal_form', for 0112's reason:
-- the class is the evidence. Those two record one employer written two ways; this records a
-- placeholder replaced by a name read from the platform, and reversing one class must not
-- touch the others.
--
-- The same DROP + re-ADD ... NOT VALID + VALIDATE shape as 0175, though the table is small:
-- validation cannot fail, since the widened list is the old list plus one value.
ALTER TABLE public.company_slug_aliases
    DROP CONSTRAINT IF EXISTS company_slug_aliases_reason;

ALTER TABLE public.company_slug_aliases
    ADD CONSTRAINT company_slug_aliases_reason CHECK (reason IN (
        'legal_form', -- a pure normalize.CompanySlug strip
        'spelling',   -- a job-count election at merge time
        'board_name'  -- a board-id placeholder replaced by the name its ATS states
    )) NOT VALID;

ALTER TABLE public.company_slug_aliases
    VALIDATE CONSTRAINT company_slug_aliases_reason;
