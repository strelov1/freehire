-- What it costs to apply — the one thing the catalogue knows nothing about.
--
-- A posting that takes one click and a posting that demands fourteen screening questions
-- with three essays are indistinguishable here today, and a candidate only finds out
-- after committing to the form. Three of the platforms already crawled hand their
-- application form over anonymously, in machine-readable shape; this is where it lands.
--
-- Applied to a fresh volume by initdb after 0071; on an existing prod volume run this
-- manually (SET ROLE hire) BEFORE deploying code that reads it. Purely additive, two new
-- tables, no column added to jobs — deliberately, since jobs is the hot table and a
-- long-running read plus an ALTER on it takes the site down.

-- One current application form per job, as the platform published it.
--
-- The payload keeps the ATS's OWN vocabulary: its field identifiers, its option values,
-- its question text, unnormalized. This is the opposite of what skilltag and classify do,
-- and on purpose — those dictionaries exist to make facets comparable ACROSS platforms,
-- whereas a form field's identifier is not for comparing. `question_67165648` means
-- nothing except to Greenhouse, and it exists to be handed back to Greenhouse. Any
-- mapping of it is loss.
--
-- Hence jsonb rather than columns: three platforms describe a form three different ways,
-- and a column set would be a schema they disagree about. job_id is the primary key, so
-- "at most one current form" is structural — a re-capture replaces rather than
-- accumulates, and no code path can produce a job with two forms.
CREATE TABLE IF NOT EXISTS apply_forms (
    job_id      bigint      PRIMARY KEY REFERENCES jobs (id) ON DELETE CASCADE,
    -- Which platform this was read from, and when. Provenance is not decoration here:
    -- captures are refreshed by dropping a provider's rows and letting the queue refill,
    -- so a platform that changes its payload shape is recoverable as a group.
    provider    text        NOT NULL,
    captured_at timestamptz NOT NULL DEFAULT now(),
    payload     jsonb       NOT NULL
);

-- No index on provider. The group re-capture it would serve is a manual, occasional
-- operator statement over a table of a few hundred thousand rows, where a sequential scan
-- is the right cost — and the read path finds a form by job_id, which the primary key
-- already answers. Add one when something runs that query often enough to need it.

-- The capture queue for the two platforms whose form costs a request.
--
-- Recruitee's form arrives inside the board listing ingest already downloads, so it is
-- written straight through and never appears here. Greenhouse and Ashby expose theirs
-- only per posting: fetching that during the crawl would turn one board request into
-- hundreds and make a crawl's duration a function of board size. Worse, an adapter cannot
-- know which postings are new — that answer only exists after UpsertJob's ON CONFLICT
-- resolves. So the write path enqueues and cmd/capture-apply-form drains.
--
-- Columns and lease semantics are semantic_outbox's, copied rather than redesigned:
-- claimed_at is a lease so a dead worker's rows return without a reaper process, attempts
-- and failed_at bound the retry, and last_error keeps the reason queryable.
CREATE TABLE IF NOT EXISTS apply_form_outbox (
    id         bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- One live entry per job — the enqueue dedup key. Unlike enrichment_outbox there is
    -- no target version in it: a form is captured once and does not have generations. A
    -- re-capture is an operator dropping the stored row, which reopens the gate.
    job_id     bigint      NOT NULL UNIQUE REFERENCES jobs (id) ON DELETE CASCADE,
    attempts   integer     NOT NULL DEFAULT 0,
    claimed_at timestamptz,
    failed_at  timestamptz,
    last_error text        NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Partial index over claimable (not dead-lettered) entries, mirroring
-- enrichment_outbox_claimable_idx and semantic_outbox_claimable_idx.
CREATE INDEX IF NOT EXISTS apply_form_outbox_claimable_idx
    ON apply_form_outbox USING btree (id) WHERE (failed_at IS NULL);
