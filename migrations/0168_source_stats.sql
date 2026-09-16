-- What each source is currently worth, measured by a pass inside cmd/rollup-stats and read
-- by the public /sources page.
--
-- Measured on that worker's own cadence, which is every 3 hours (not daily): it already
-- runs several full-table aggregates over `jobs` on that schedule, so this narrow scan
-- rides a warm page cache rather than adding a new sweep to the host.
--
-- A SNAPSHOT, not a ledger: one row per source, replaced on every run. Keeping the
-- history would invite a reader to plot a trend off a figure nobody maintains, and
-- nothing has asked for one.
--
-- TWO job counts on purpose, the same pair catalogstats already carries for the
-- catalogue as a whole, and for the same reason:
--
--   open_jobs        the raw count of that source's open postings. This is what the
--                    ATS-overlap arithmetic is computed from — ats_matched_jobs is a
--                    count of ROWS carrying jobs.duplicate_of_aggregator, so matched
--                    plus unmatched must equal the raw row count. A de-duplicated
--                    denominator would leave the two halves not adding up.
--
--   browsable_jobs   what Meilisearch's own de-duplicated index holds for the source,
--                    which is what /jobs?source=<key> will actually show. This is the
--                    figure the page displays, because a card reading 12,000 beside a
--                    link that opens 4,000 is the trap this pair exists to avoid.
--
-- Both are written in the same statement from the same run, so the page can never
-- render two figures that describe different moments.
--
-- browsable_jobs is NULLABLE and null means NOT MEASURED — an absent or unreachable
-- Meilisearch costs that one figure and not the run. It must never be written as 0:
-- "we could not measure this" and "this source has nothing" are different answers,
-- and only one of them belongs on a public page.
--
-- ats_matched_jobs is NOT NULL and is meaningful only for aggregator sources. The
-- dedup pass sets duplicate_of_aggregator solely on an aggregator row that matched a
-- non-aggregator one, so a first-party ATS source's 0 here is arithmetic, not a
-- finding — the endpoint omits the figure entirely for non-aggregators rather than
-- letting that 0 read as "fully exclusive".
--
-- sample_url is one of the source's own posting URLs, taken with min(url) in the same
-- grouped scan: its HOST is what the logo proxy resolves a brand mark from. Derived
-- rather than hand-listed because a map of 221 domains goes stale silently and the
-- entry it is missing is invisible. NULL when the source has no open posting, which
-- is the honest outcome — no postings, no evidence of a host, no logo.
CREATE TABLE public.source_stats (
    source           text                     NOT NULL,
    open_jobs        bigint                   NOT NULL,
    ats_matched_jobs bigint                   NOT NULL,
    browsable_jobs   bigint,
    sample_url       text,
    measured_at      timestamp with time zone NOT NULL DEFAULT now(),
    CONSTRAINT source_stats_pkey PRIMARY KEY (source)
);

-- No index beyond the primary key. The only read is "give me every row" — the table
-- is one row per registered adapter, on the order of a few hundred — and the only
-- write is an upsert keyed on the primary key.
