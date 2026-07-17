-- Precomputed facet-distribution snapshot for the public /open transparency page.
-- Unlike the insights_* rollups (0022), whose authoritative source is the `jobs`
-- table, this snapshot's source is Meilisearch's facet count: cmd/rollup-facets
-- calls the same search.FacetCounts the live filters use, so the snapshot's numbers
-- match the live catalogue and the SPA filter facets exactly. It is recomputed as an
-- atomic delete-and-reinsert inside one transaction (never an upsert), so a reader
-- never sees a partially rebuilt table.
--
-- One row per (facet, value): `facet` is a public facet param name — only the four
-- /open renders (`countries`, `skills`, `seniority`, `work_mode`) — and `value` is
-- that facet's value with its `count` of matching open jobs. The full distribution
-- is stored; top-N is sliced on read.
--
-- Applied to a fresh volume by initdb after 0025; on an existing prod volume this
-- CREATE statement must be run manually BEFORE deploying code that reads it (per the
-- migrations gotcha).

CREATE TABLE public.insights_facet_stats (
    facet text   NOT NULL,
    value text   NOT NULL,
    count bigint NOT NULL,
    PRIMARY KEY (facet, value)
);

-- Reads select one facet slice and order by count DESC to take the top-N; this
-- index serves that ranked read.
CREATE INDEX insights_facet_stats_facet_count_idx
    ON public.insights_facet_stats (facet, count DESC);
