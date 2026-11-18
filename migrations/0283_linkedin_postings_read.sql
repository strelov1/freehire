-- Negative cache of LinkedIn postings whose detail was read and found NOT on-site (external or
-- unknown application method). Such a posting is never stored as a jobs row (it goes to native
-- ATS resolution), so the seen-set could not tell the crawl it was already read, and every
-- crawl paid a detail request for it again. A row here makes the posting read as seen until
-- read_at ages past the body-refresh window. posting_id is the bare numeric LinkedIn id, the
-- same key LinkedIn rows use in jobs.external_id. Rows older than the retention window are
-- pruned by the seen-set lookup; the table stays small.
CREATE TABLE linkedin_postings_read (
    posting_id   text        PRIMARY KEY,
    apply_method text        NOT NULL,
    read_at      timestamptz NOT NULL
);

CREATE INDEX linkedin_postings_read_read_at_idx ON linkedin_postings_read (read_at);
