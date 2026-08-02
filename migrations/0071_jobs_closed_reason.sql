-- Which mechanism closed a job, recorded next to closed_at.
--
-- Five paths write closed_at today — the unseen sweep (both the company-scoped and the
-- source-scoped variant), a feed's removal event, a moderator, and the liveness probe — and
-- the column says the same word for all of them. A sixth is arriving that is unlike the
-- others: an age rule for sources carrying no close signal at all, which closes on a guess
-- rather than on evidence. Folding "we presume this went stale" into the same undifferentiated
-- "closed" would make the signal unreadable, and this project has already declined that trade
-- once: catalogue pruning got its own pruned_jobs table rather than a second meaning for
-- closed_at, because overloading it "would corrupt a signal three mechanisms already write".
--
-- Recording the mechanism per row is the same discipline pruned_jobs applies to its rules
-- (0041): a rule that turns out to be too broad can then be audited, and undone, alone.
--
-- Applied to a fresh volume by initdb after 0070; on an existing prod volume run this
-- manually (SET ROLE hire) BEFORE deploying code that writes it. Additive with a default,
-- so no table rewrite and no backfill.

-- '' is the sixth permitted value and is NOT a placeholder for a missing one: every row
-- that exists when this migration runs takes it, and those closes genuinely have no
-- recorded mechanism. Inferring one after the fact would invent history — a row closed
-- last month cannot be shown to have been closed by the sweep rather than by a moderator.
-- A CHECK that omitted '' would reject the entire existing table.
ALTER TABLE public.jobs
    ADD COLUMN closed_reason text NOT NULL DEFAULT '';

ALTER TABLE public.jobs
    ADD CONSTRAINT jobs_closed_reason_check CHECK (closed_reason = ANY (ARRAY[
        ''::text,               -- closed before this column existed; unknown
        'unseen'::text,         -- dropped off a board we crawl (CloseUnseenJobs / …BySource)
        'feed_removed'::text,   -- a streaming source reported it taken down
        'moderated'::text,      -- a moderator closed it
        'probe_expired'::text,  -- two consecutive expired liveness probes
        'expired'::text         -- age rule: no close signal exists for this source
    ]));
