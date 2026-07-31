-- An append-only, content-free record of what happened to an application.
--
-- The facts a job search produces currently live in mutable columns and are forgotten as
-- they are overwritten: user_jobs.stage has no transition date, user_jobs.followed_up_at
-- holds one timestamp so a second chase erases the first, and emails.job_id moves when a
-- link is confirmed, corrected, or undone. On top of those sits a public claim — the
-- per-company response rate — recomputed from live mail rows WHERE deleted_at IS NULL, so
-- a candidate tidying their inbox made a named employer look more silent than it was.
--
-- This table is the record that stops changing once written. It holds no subject lines,
-- no bodies, no addresses: only that something happened, when, and to which application.
--
-- Applied to a fresh volume by initdb after 0059; on an existing prod volume run this
-- manually (SET ROLE hire) BEFORE deploying code that reads it. Additive, no backfill —
-- cmd/backfill-application-events populates it as its own pass afterwards.

CREATE TABLE IF NOT EXISTS application_events (
    id           bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id      bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- The application this happened to. ON DELETE SET NULL mirrors emails.job_id: a job
    -- removed by cmd/prune must not cascade-delete the history of applying to it.
    job_id       bigint      REFERENCES jobs (id) ON DELETE SET NULL,
    -- Denormalized at write time, and load-bearing for exactly that reason. cmd/prune is
    -- the only hard-delete path for jobs; once it clears job_id above, an event that had
    -- to join jobs for its company would drop out of the company aggregate retroactively
    -- — the instability this table exists to remove.
    company_slug text        NOT NULL DEFAULT '',
    -- What happened: applied | employer_reply | follow_up_sent | stage_set. Kept separate
    -- from signal because this vocabulary is ours, narrow, and meant to stay still.
    kind         text        NOT NULL,
    -- What was in it: the mailclassify status vocabulary for employer_reply, the stage for
    -- stage_set. That vocabulary has grown before and will again; folding it into kind
    -- would make every classifier addition a change to a table that must not change.
    signal       text        NOT NULL DEFAULT '',
    -- When it happened — emails.received_at for mail-derived events. All day arithmetic
    -- reads this column.
    occurred_at  timestamptz NOT NULL,
    -- When we learned it. Connecting a mailbox imports a year of ATS mail in one pass, so
    -- a ledger keyed on write time would report every past reply arriving that day.
    recorded_at  timestamptz NOT NULL DEFAULT now(),
    -- Where it came from: mail_gmail | mail_hosted | mail_external | user | assistant.
    -- Only the mail sources carry an objective timestamp; a manually-set stage records
    -- when the candidate got around to updating their board, which is why stage_set is
    -- written from day one and read by no timing yet.
    source       text        NOT NULL,
    -- The record that produced it (emails.id for mail-derived events), NULL for manual
    -- ones. Also the idempotency key — see the unique index below.
    source_ref   bigint,
    -- Set when a link correction moves the fact to another employer. Deleting the message
    -- does NOT set this: deletion says the candidate does not want to see it, re-linking
    -- says the fact belongs elsewhere. The row survives either way — an event recorded in
    -- error is itself a fact, and this table is append-only.
    retracted_at timestamptz
);

-- The account-deletion cascade and every per-user read. Deliberately NOT partial: the
-- unique index below also leads with user_id and would satisfy
-- TestEveryUserForeignKeyIsIndexed, but its WHERE clause excludes manual events, so the
-- cascade would still scan for them.
CREATE INDEX IF NOT EXISTS application_events_user_occurred_idx
    ON application_events (user_id, occurred_at);

-- Idempotency by constraint rather than by coordination: emission and backfill become the
-- same operation, so cmd/classify-mail and cmd/backfill-application-events can meet on the
-- same email in any order and produce one row. Manual events carry no source_ref and sit
-- outside the index — two consecutive follow-ups are two facts, not a duplicate.
CREATE UNIQUE INDEX IF NOT EXISTS application_events_source_ref_key
    ON application_events (user_id, kind, source_ref) WHERE source_ref IS NOT NULL;

-- "Did this application ever get a reply?" — the aggregate's per-application question.
CREATE INDEX IF NOT EXISTS application_events_app_idx
    ON application_events (user_id, job_id, kind) WHERE retracted_at IS NULL;

-- The per-company rollup's grouping key.
CREATE INDEX IF NOT EXISTS application_events_company_idx
    ON application_events (company_slug, kind) WHERE retracted_at IS NULL;
