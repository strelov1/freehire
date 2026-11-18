-- migrate: no-transaction
--
-- Adds 'moderator' to company_slug_aliases.reason's allowed values (see 0112 and 0195). It
-- labels a merge a moderator approved from a user's proposal (company_relation_proposals,
-- migration 0229): two slugs a person knows are one employer and the name fold cannot reach.
--
-- It needs its own label for 0112's reason: the class is the evidence. 'legal_form' is a
-- rule, 'spelling' an election by job count, 'board_name' a name read from the ATS. This one
-- is a human judgement, and reversing it must not touch the others.
--
-- The same DROP + re-ADD ... NOT VALID + VALIDATE shape as 0195. Validation cannot fail,
-- since the widened list is the old list plus one value.
ALTER TABLE public.company_slug_aliases
    DROP CONSTRAINT IF EXISTS company_slug_aliases_reason;

ALTER TABLE public.company_slug_aliases
    ADD CONSTRAINT company_slug_aliases_reason CHECK (reason IN (
        'legal_form', -- a pure normalize.CompanySlug strip
        'spelling',   -- a job-count election at merge time
        'board_name', -- a board-id placeholder replaced by the name its ATS states
        'moderator'   -- a moderator-approved proposal (company_relation_proposals)
    )) NOT VALID;

ALTER TABLE public.company_slug_aliases
    VALIDATE CONSTRAINT company_slug_aliases_reason;

-- The approval refuses to retire a slug that others already redirect to, and so asks
-- "is this anyone's canon" on every proposal and approval. Without an index that is a scan of
-- the registry each time. InsertMissingCanonicalCompanies reads the same column.
--
-- A plain build, not CONCURRENTLY. The table holds ~56k rows, so the build takes well under
-- a second, and its SHARE lock conflicts only with writes to this table. CONCURRENTLY also
-- waits for every older transaction in the whole database. On prod (2026-11-03) that wait
-- ran into migrate's lock_timeout and left the index INVALID, which failed the release.
-- squawk-ignore require-concurrent-index-creation -- a ~56k-row table; see above
CREATE INDEX IF NOT EXISTS company_slug_aliases_canonical_slug_idx
    ON company_slug_aliases (canonical_slug);
