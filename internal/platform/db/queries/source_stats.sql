-- Per-source snapshot (source_stats): the measurement cmd/rollup-stats takes on each run
-- and the read the public /api/v1/sources endpoint serves from it.
--
-- Rebuilt as an atomic delete-and-reinsert inside one transaction, like the facet
-- snapshot beside it, so a reader never sees a partially rebuilt table — and so an
-- adapter removed from the registry leaves the snapshot instead of lingering as a row
-- nothing can explain.

-- name: AggregateOpenJobsBySource :many
-- The whole measurement in one grouped scan over open postings.
--
-- Reads no description column, deliberately: a `description` predicate de-TOASTs every
-- row it touches, which at this catalogue's size is the difference between a pass that
-- runs on a schedule and one that never finishes. Everything here is narrow.
--
-- ats_matched counts rows the dedup pass matched to a first-party ATS posting. The
-- marker is only ever set on an AGGREGATOR row (see the aggregator-suppression pass),
-- so this is 0 for every other kind of source by construction — which is why the
-- endpoint omits the figure for them rather than publishing that 0.
--
-- It does NOT sample a posting URL. The first version did, to resolve a logo from the
-- host — on the assumption that every posting of a source shares one. Production disproved
-- it: an ATS posting's URL usually lives on the EMPLOYER's domain, so greenhouse sampled
-- bankrate.com and successfactors a staffing agency, and the page served the wrong brand.
-- Logos are resolved from the source's display name client-side instead.
--
-- NOT is_private excludes the jd-tailor-intake private postings: one user's pasted job
-- description, visible only to them. They are not part of the catalogue, they are already
-- excluded from the search index at enqueue, and counting them here would both inflate a
-- public figure and make it disagree with the de-duplicated count measured beside it.
--
-- No index backs this. It is one sequential scan per rollup-stats run (every 3 hours, on a
-- worker that already sweeps `jobs` several times per run), and an index on jobs(source)
-- would be built and maintained on an 11M-row table to serve exactly one query.
SELECT source,
       count(*)::bigint                                                    AS open_jobs,
       (count(*) FILTER (WHERE duplicate_of_aggregator IS NOT NULL))::bigint AS ats_matched_jobs
FROM jobs
WHERE closed_at IS NULL AND NOT is_private
GROUP BY source;

-- name: DeleteAllSourceStats :exec
-- First half of the atomic rebuild. Run in the same transaction as the
-- InsertSourceStat loop.
DELETE FROM source_stats;

-- name: InsertSourceStat :exec
-- Second half of the atomic rebuild: one row per source in the UNION of the adapter
-- registry and what the scan above found — see sourcestats.Rows, which is where that
-- union is decided.
--
-- Not "one per scanned source": a registered adapter whose postings have all closed must
-- land here carrying 0, because "we read this source and it currently carries nothing" is
-- a measurement, while a missing row would be read as "we never measured it", and the
-- page says different things about the two.
--
-- Not "one per registered adapter" either: a source can carry postings without being a
-- crawl adapter (telegram), and dropping it would quietly falsify a page whose whole
-- claim is that it lists every source.
--
-- browsable_jobs is NULL when Meilisearch could not be reached. Never 0: see the
-- migration's comment.
INSERT INTO source_stats (source, open_jobs, ats_matched_jobs, browsable_jobs, measured_at)
VALUES (
    sqlc.arg('source'),
    sqlc.arg('open_jobs'),
    sqlc.arg('ats_matched_jobs'),
    sqlc.narg('browsable_jobs'),
    sqlc.arg('measured_at')
);

-- name: ListSourceStats :many
-- The whole snapshot. Aggregate only — per-source counts, no record-level data. A few hundred rows, so it is read whole and joined in Go against
-- the adapter registry rather than filtered here.
SELECT source, open_jobs, ats_matched_jobs, browsable_jobs, measured_at
FROM source_stats
ORDER BY source;
