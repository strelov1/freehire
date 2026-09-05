-- The daily social digest: what it ranks on, and what it remembers publishing.
--
-- 0037 gave job_daily_views a single `uniques` column that fuses the two signals
-- cmd/rollup-views counts — a page open (GET /jobs/<slug>, filtered against the
-- known-bot list in internal/application/viewlog/bot.go) and an API read
-- (GET /api/v1/jobs/<slug>, deliberately NOT bot-filtered, because the API exists
-- to be read by programs). For the catalogue-scale figure that column serves today
-- the fusion is harmless. For ranking a post we publish under our own name it is
-- not: crawlers are the majority of this host's traffic, and that bot list is
-- deliberately small — it errs toward counting a person rather than toward
-- excluding a crawler. A "most viewed today" list built on `uniques` would be a
-- list of what crawlers fetched, shown to humans as what humans liked.
--
-- So `page_uniques` is added BESIDE `uniques` rather than redefining it. `uniques`
-- keeps its meaning, its value, and its reader (GET /api/v1/stats/catalog via
-- internal/ingest/catalogstats). Redefining it would silently restate a public
-- figure and would need a backfill to make months of existing rows agree with the
-- new meaning; adding a column changes nothing that already works.
--
-- NOT backfilled. Every pre-existing row holds 0 and stays there. The digest only
-- ever reads the freshest day, so it is correct from the first cmd/rollup-views run
-- after deploy. Re-reading the .gz log history to fill a column nothing queries
-- would be work for its own sake — and if some later consumer does want the split
-- historically, `rollup-views --backfill` against a cleared cursor is the path.
ALTER TABLE public.job_daily_views
    -- Matches the type of `uniques` beside it. A per-job, per-day unique count is
    -- tens or hundreds, and the two are compared against each other (page_uniques
    -- can never exceed uniques), so they must not disagree about their width.
    -- squawk-ignore prefer-bigint-over-int -- mirrors job_daily_views.uniques (0037)
    ADD COLUMN page_uniques integer NOT NULL DEFAULT 0;

COMMENT ON COLUMN public.job_daily_views.page_uniques IS
    'Unique daily visitors counted from PAGE opens only, which are bot-filtered. '
    'The `uniques` column beside it fuses page opens with API reads, and API reads '
    'carry no bot filtering — so page_uniques is the only one of the two that '
    'describes people, and the only one safe to rank a public post on. Zero for '
    'every row written before migration 0138; deliberately not backfilled.';

-- The digest's ledger: what was published, where, and when.
--
-- It answers two different questions, which is why it is one table and not two:
--
--   1. "Has this (day, channel) already gone out?" — the publish-once guarantee.
--      Keyed on the channel and not on the day alone, because a run that posts to
--      Discord and then fails on LinkedIn must, on its next attempt, skip Discord
--      and retry LinkedIn. A day-only key would either re-post to Discord or
--      abandon LinkedIn, and both are wrong.
--
--   2. "Was this posting in a digest recently?" — the quarantine, which keeps a
--      popular posting from being the lead item every day for a week. It reads
--      across channels on purpose: the list is the editorial unit, the channel is
--      only how it is delivered, so a posting shown on Discord yesterday is
--      quarantined for LinkedIn today too.
--
-- No foreign key to jobs. cmd/prune is the only hard-delete path in this system and
-- it archives to pruned_jobs; a ledger of what we said in public should survive the
-- posting it referred to, the same way a sent email does. A row whose job_id no
-- longer resolves simply stops matching the quarantine join, which is the correct
-- behaviour — a pruned posting cannot be re-published anyway.
CREATE TABLE public.social_digest_posts (
    -- The day the digest DESCRIBES, not the day it was sent. Those differ by at
    -- least one: the freshest day in job_daily_views is always a completed day.
    day          date        NOT NULL,
    -- 'discord' today. Text and not an enum: adding a channel should be a Go change
    -- and a config line, not a migration — which is also why this column exists while
    -- only one channel does. A ledger keyed on the day alone would make the second
    -- destination a schema change, and the second destination is a matter of when.

    channel      text        NOT NULL,
    job_id       bigint      NOT NULL,
    -- Where the posting sat in the published list. Neither question above needs it;
    -- it is here so the ledger can reconstruct the post that went out, which is what
    -- lets this table stand in for an archive surface we are deliberately not
    -- building. Named `slot` rather than `rank` to stay clear of the window function.
    -- squawk-ignore prefer-bigint-over-int -- a position within a list of ten
    slot         int         NOT NULL,
    published_at timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (day, channel, job_id)
);

-- No second index. Both questions above are answered by the primary key's leading
-- `day`: the publish-once check is an equality on (day, channel), and the quarantine
-- is `SELECT DISTINCT job_id WHERE day >= $1 AND day < $2` — a range scan that never
-- leaves the index. A table this small does not deserve an index nothing reads.

COMMENT ON TABLE public.social_digest_posts IS
    'Ledger of published daily social digests. Unique on (day, channel, job_id): the '
    'publish-once check reads the (day, channel) prefix, the quarantine scans a day '
    'RANGE across all channels — [digest day - 7, digest day), the upper bound '
    'exclusive so that a digest cannot quarantine itself. Written only after a '
    'channel publishes successfully; a dry run never writes here.';
