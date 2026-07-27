-- Catalogue pruning keeps a record of what it permanently removed. The campaign
-- deletes roughly 1.5M jobs that do not belong on an IT job board, and the whole point
-- of deleting rather than soft-closing is to reclaim the disk their description and
-- enrichment occupy — so this table deliberately carries neither. What it does carry is
-- enough to answer the one question an irreversible deletion otherwise makes
-- unanswerable: was something removed that should have been kept.
--
-- No foreign key to jobs: the referenced row is gone by construction. The primary key
-- is the deleted job's own id, which jobs never reuses (identity always generated), so
-- an entry can never be overwritten by a later posting.
CREATE TABLE public.pruned_jobs (
    id           bigint      NOT NULL PRIMARY KEY,
    source       text        NOT NULL,
    external_id  text        NOT NULL,
    title        text        NOT NULL,
    company_slug text        NOT NULL,
    -- Which rule matched: the blue-collar title blocklist, a business role at a company
    -- with no technical evidence, or an unclassified job at a company with none at all.
    -- Recorded per row so a rule that turns out to be too broad can be audited alone.
    rule         text        NOT NULL,
    pruned_at    timestamp with time zone NOT NULL DEFAULT now()
);
