-- name: DuplicateClosureGeoAll :many
-- The whole-catalogue geography union in one pass, keyed by the id of the row that OWNS each
-- union — the searchable canonical row, widened with the geography of every OPEN row it
-- represents. What a row represents is its DUPLICATE CLOSURE: the rows whose duplicate_of
-- chain terminates at it, at any depth.
--
-- The closure, not a shared role_fingerprint, is the membership rule, because a fingerprint
-- cannot express what two of the three dedup passes do. The exact role pass clusters BY
-- fingerprint, so its members are inside their canon's closure and its behaviour is unchanged;
-- but the fuzzy-description and aggregator passes only ever act on rows the exact pass left
-- unclustered, whose fingerprints therefore DIFFER from their canon's by construction. Keyed
-- by fingerprint, their members' cities left the index with them — issue #2225, where a
-- posting open in Chestermere was readable by slug and absent from every search.
--
-- A chain of OPEN rows (a role canon that a later pass suppressed) resolves to its ultimate
-- owner, so no member's geography is stranded on an open intermediate row.
--
-- A CLOSED intermediate cuts the walk, and that is deliberate: the traversal follows open rows
-- only, so an open row behind a closed parent contributes to nobody. Such a row is invisible
-- either way — it carries a marker, so it is out of the index, and so is the closed row it
-- points at. Re-pointing it is the marker refresh's job, not this read's: the role recompute
-- picks min(id) among a cluster's OPEN rows and the fuzzy pass releases a marker whose canon
-- closed. Measured on prod 2026-09-01, 42 633 open duplicates sat behind a closed owner —
-- which is the never-released fuzzy marker this change also fixes, not a gap here.
--
-- Cycle safety is structural, not a guard. Each row has at most ONE duplicate_of, so the
-- "points at" graph has out-degree <= 1; a cycle in such a graph consists entirely of rows
-- with duplicate_of set, and every edge INTO a cycle member comes from another cycle member.
-- Seeding only from rows that are nobody's duplicate (duplicate_of IS NULL) therefore makes a
-- cycle unreachable rather than merely survivable — which is why BOTH closure queries seed
-- that way. The depth bound is a backstop for a future caller that seeds differently; today's
-- chains are at most role -> fuzzy on top of aggregator, three hops.
--
-- Only owners that represent at least one other open row are returned: a row representing
-- nobody unions to its own geography, which MergeClusterGeography already treats as a no-op,
-- and leaving it out is what keeps the rebuild's lookup map to the rows that need it. The
-- LATERAL tags each member's countries/regions/cities into one unnested stream (no cartesian
-- across the three arrays, no repeat self-join), and blanks are dropped by the FILTER.
--
-- The DISTINCT runs in a subquery, ahead of the aggregate, for what it costs: array_agg(DISTINCT)
-- sorts every unnested value, about 160M rows on prod against a few million distinct ones, and
-- that sort spilled to disk for over four hours of a rebuild (2026-11-05). A hash aggregate over
-- the distinct values does the same work in memory, and the answer is unchanged.
WITH RECURSIVE member AS (
    SELECT o.id AS owner_id, o.id AS member_id, 0 AS depth
    FROM jobs o
    WHERE o.closed_at IS NULL
      AND o.duplicate_of IS NULL
      AND EXISTS (SELECT 1 FROM jobs c WHERE c.duplicate_of = o.id AND c.closed_at IS NULL)
    UNION ALL
    SELECT m.owner_id, c.id, m.depth + 1
    FROM member m
    JOIN jobs c ON c.duplicate_of = m.member_id
    WHERE c.closed_at IS NULL AND m.depth < 8
)
SELECT
    g.owner_id,
    array_agg(g.val ORDER BY g.val) FILTER (WHERE g.kind = 'c' AND g.val <> '')::text[] AS countries,
    array_agg(g.val ORDER BY g.val) FILTER (WHERE g.kind = 'r' AND g.val <> '')::text[] AS regions,
    array_agg(g.val ORDER BY g.val) FILTER (WHERE g.kind = 'y' AND g.val <> '')::text[] AS cities,
    array_agg(g.val ORDER BY g.val) FILTER (WHERE g.kind = 'm' AND g.val <> '')::text[] AS metros
FROM (
    SELECT DISTINCT m.owner_id, t.kind, t.val
    FROM member m
    JOIN jobs o ON o.id = m.member_id
    LEFT JOIN LATERAL (
        SELECT 'c'::text AS kind, e AS val FROM unnest(o.countries) AS e
        UNION ALL
        SELECT 'r', e FROM unnest(o.regions) AS e
        UNION ALL
        SELECT 'y', e FROM unnest(o.cities) AS e
        UNION ALL
        SELECT 'm', e FROM unnest(o.metros) AS e
    ) t ON true
) g
GROUP BY g.owner_id;

-- name: DuplicateClosureGeoFor :many
-- The duplicate-closure geography union for a SPECIFIC set of owner ids, so an incremental
-- index push can widen a whole wave's rows in one query instead of one call per job.
--
-- The recursive body below is a COPY of DuplicateClosureGeoAll's, not a shared one — sqlc
-- names whole statements, so there is nowhere to put it once. The walk, the open-rows-only
-- scope, the closed-intermediate behaviour and the depth bound are argued there. What must
-- stay identical is the recursive term and the seed's `closed_at IS NULL AND duplicate_of IS
-- NULL`: change either here alone and the wave's answer stops matching the rebuild's, which
-- surfaces as a canon that silently narrows every time the drain touches it.
--
-- One deliberate difference: the seed carries no EXISTS test. The caller named these rows, and
-- a row representing nobody answers with its own geography — a self-union, and a documented
-- no-op merge. That keeps the caller to one error branch instead of making it tell "this row
-- owns nothing" apart from a failure.
--
-- An id matching no open canonical row simply yields no row for that id, which the caller
-- reads as "no widening".
WITH RECURSIVE member AS (
    SELECT o.id AS owner_id, o.id AS member_id, 0 AS depth
    FROM jobs o
    WHERE o.id = ANY(sqlc.arg(owner_ids)::bigint[])
      AND o.closed_at IS NULL
      AND o.duplicate_of IS NULL
    UNION ALL
    SELECT m.owner_id, c.id, m.depth + 1
    FROM member m
    JOIN jobs c ON c.duplicate_of = m.member_id
    WHERE c.closed_at IS NULL AND m.depth < 8
)
SELECT
    m.owner_id,
    array_agg(DISTINCT t.val) FILTER (WHERE t.kind = 'c' AND t.val <> '')::text[] AS countries,
    array_agg(DISTINCT t.val) FILTER (WHERE t.kind = 'r' AND t.val <> '')::text[] AS regions,
    array_agg(DISTINCT t.val) FILTER (WHERE t.kind = 'y' AND t.val <> '')::text[] AS cities,
    array_agg(DISTINCT t.val) FILTER (WHERE t.kind = 'm' AND t.val <> '')::text[] AS metros
FROM member m
JOIN jobs o ON o.id = m.member_id
LEFT JOIN LATERAL (
    SELECT 'c'::text AS kind, e AS val FROM unnest(o.countries) AS e
    UNION ALL
    SELECT 'r', e FROM unnest(o.regions) AS e
    UNION ALL
    SELECT 'y', e FROM unnest(o.cities) AS e
    UNION ALL
    SELECT 'm', e FROM unnest(o.metros) AS e
) t ON true
GROUP BY m.owner_id;
