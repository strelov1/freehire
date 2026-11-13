-- name: RoleClusterCount :one
-- The job-reality repost/mass-posting counts for one role cluster: how many postings
-- of the same role (by role_fingerprint within a company) exist of any status
-- (repost_count = repost history) and how many are still open (mass_count = concurrent
-- mass-posting). A NULL/empty fingerprint is excluded so unfingerprinted rows never
-- cluster together; a lookup miss means a unique role (count 1). Used by the
-- incremental index push and the single-job detail read.
SELECT
    COUNT(*)::bigint AS repost_count,
    COUNT(*) FILTER (WHERE closed_at IS NULL)::bigint AS mass_count
FROM jobs
WHERE company_slug = sqlc.arg(company_slug)
  AND role_fingerprint = sqlc.arg(role_fingerprint)
  AND role_fingerprint <> '';

-- name: CompanyHasOtherJobs :one
-- Whether the catalog carries this company beyond the one posting named by (source,
-- external_id) — asked right after an import, to answer "is this company new to us?".
-- The board-level check (BoardTracked) cannot answer it: a company reached through an
-- ATS we do not recognise, or through a second ATS, is still a company we already carry.
-- The posting itself is excluded by its dedup identity because it is written before this
-- runs. Callers skip the question for an empty company_slug, which names nobody.
SELECT EXISTS (
    SELECT 1
    FROM jobs
    WHERE company_slug = sqlc.arg(company_slug)
      AND NOT (source = sqlc.arg(source) AND external_id = sqlc.arg(external_id))
) AS carried;

-- name: CompaniesWithFreshNonAggregatorCoverage :many
-- The ingest-time aggregator coverage gate: of the companies asked about, which still have an
-- OPEN posting from a non-aggregator source that was seen recently. A yes makes ingest DISCARD
-- the aggregator's posting unsaved, so the question has to be about the present tense — "do we
-- still crawl this employer" — not "did we ever". Without the last_seen_at cutoff a single
-- forgotten row holds its slug forever: a 2013 trakstar posting from a board that had left
-- sources/ suppressed every live Himalayas posting for a different employer of the same name
-- (issue #2315), and 22,022 slugs were held that way on prod 2026-09-02.
--
-- Companies arrive ALREADY FOLDED and are compared against the stored jobs.company_slug_folded
-- column, which UpsertJob writes beside company_slug. No exact-slug clause sits beside it
-- because none is needed: the fold is replace(company_slug,'-',''), so an exact match implies
-- a folded one. That is also what keeps the predicate on an index — see the note in
-- RecomputeAggregatorDuplicates about what the expression form costs the planner.
--
-- company_slug <> '' is not a filter on meaning (an empty slug names nobody) but the partial
-- index's own predicate, without which jobs_open_company_slug_folded_col_idx cannot be used.
--
-- NOT is_private is a correctness clause, not an inherited one. A private posting is the
-- jd-tailor-intake path — a job description a single user pasted in, visible only to them and
-- never crawled from anywhere. It can never be evidence that the catalogue still crawls an
-- employer, and counting it would let one user's pasted JD for "Acme" silently discard every
-- aggregator posting for every other Acme. The previous search-backed lookup excluded these by
-- accident (cmd/reindex drops is_private rows from the index entirely); reading the table
-- directly makes the exclusion something this query has to state.
--
-- The index's OTHER exclusions are deliberately NOT mirrored here. A non-aggregator row marked
-- duplicate_of is a repost of another posting of the same employer, and an uncategorised or
-- body-less one is still a posting we crawled from that employer's own board — all three are
-- real coverage. They are absent from the index because they are not worth SEARCHING, which is
-- a different question from whether we still crawl the company.
--
-- last_seen_at is job_activity's and carries NO index, and must not gain one: the re-crawl's
-- liveness write updates that column alone precisely because it is unindexed, which keeps the
-- update heap-only. It is a recheck here, over the few open rows the folded index already
-- selected (one primary-key probe on job_activity each), and EXISTS stops at the first fresh one
-- so a large employer costs no more than a small one.
--
-- countries scopes the answer to a national-market aggregator's countries (sources.MarketCountries;
-- empty = unscoped): a covering row must hire in one of them or state no country, the same rule
-- SuppressAggregatorDuplicatesForCompanies matches twins by. Matching on the name alone let a
-- foreign employer of the same name discard a local one's postings (issue #4412).
SELECT asked.folded::text AS company_slug_folded
FROM unnest(sqlc.arg(folded_companies)::text[]) AS asked(folded)
WHERE EXISTS (
    SELECT 1
    FROM jobs
    JOIN job_activity a ON a.job_id = jobs.id
    WHERE jobs.company_slug_folded = asked.folded
      AND jobs.closed_at IS NULL
      AND jobs.company_slug <> ''
      AND NOT jobs.is_private
      AND a.last_seen_at > sqlc.arg(seen_after)
      AND NOT (jobs.source = ANY(sqlc.arg(aggregators)::text[]))
      AND (cardinality(coalesce(sqlc.arg(countries)::text[], '{}')) = 0
           OR cardinality(jobs.countries) = 0
           OR jobs.countries && sqlc.arg(countries)::text[])
);

-- name: CanonicalJobForRole :one
-- The open canonical posting of one role cluster, asked by the import BEFORE it writes a
-- posting under the URL-keyed generic identity: a careers storefront on a company's own domain
-- fronts an ATS board we crawl, and without this the same vacancy lands twice — once from the
-- crawl, once from the pasted link. Mirrors the canon RecomputeRoleDuplicatesForCompany picks
-- (MIN(id) among the cluster's open rows) so this answer and the one a later reindex computes
-- agree rather than fight.
-- A canon must be open AND not itself a duplicate, or marking would build a chain (A -> B -> C)
-- that no reader expects. The row being written is excluded by its own dedup identity, because
-- a re-import of the same URL would otherwise find itself. Served by the partial
-- jobs_open_role_cluster_idx (migration 0013), with jobs_company_role_fingerprint_idx as the
-- non-partial fallback.
-- A canon must not be private either. InsertPrivateJob derives company_slug and
-- role_fingerprint like every other write path, so a pasted JD can land in a public
-- posting's role cluster; electing it here would mark the incoming public posting a
-- duplicate of a row nothing lists, removing BOTH from search — the import's own
-- posting and the private one it was pointed at.
SELECT id, public_slug
FROM jobs
WHERE company_slug = sqlc.arg(company_slug)
  AND role_fingerprint = sqlc.arg(role_fingerprint)
  AND closed_at IS NULL
  AND duplicate_of IS NULL
  AND NOT is_private
  AND NOT (source = sqlc.arg(source) AND external_id = sqlc.arg(external_id))
ORDER BY id
LIMIT 1;

-- name: MarkJobDuplicateOfRole :one
-- Point one row at its ROLE canon. The import path only: the batch passes recompute whole
-- companies (RecomputeRoleDuplicatesForCompanies, SuppressAggregatorDuplicatesForCompanies) and
-- must keep doing so — this marks the single row an import just wrote.
--
-- Writes duplicate_of_role, not duplicate_of, and the name says so. Both callers
-- (cmd/ingest/store.go, internal/ingest/linkimport) resolve their canon through
-- jobdedup.CanonicalForRole, so this is the role verdict arriving early — the same clustering
-- the batch pass would reach hours later. A write to duplicate_of itself would not survive:
-- the derivation in migration 0115 recomputes it from the owned columns.
WITH before AS (
    SELECT b.id, b.duplicate_of AS was_duplicate_of FROM jobs b WHERE b.id = sqlc.arg(id)
),
updated AS (
    UPDATE jobs j
    SET duplicate_of_role = sqlc.arg(duplicate_of_role),
        updated_at        = now()
    WHERE j.id = sqlc.arg(id)
    RETURNING j.id,
              j.duplicate_of AS now_duplicate_of,
              COALESCE(j.posted_at, j.created_at) AS eff_posted_at
),
flipped AS (
    SELECT u.id, b.was_duplicate_of, u.now_duplicate_of, u.eff_posted_at
    FROM updated u JOIN before b ON b.id = u.id
),
dequeued AS (
    INSERT INTO search_delete_outbox (job_id)
    SELECT id FROM flipped WHERE was_duplicate_of IS NULL AND now_duplicate_of IS NOT NULL
    ON CONFLICT (job_id) DO NOTHING
),
-- Both callers pass a canon, so today this branch never fires: the import path only ever SETS
-- the role marker. It is here because the argument is nullable and the other three writers are
-- symmetric — a future caller clearing the marker through this query would otherwise leave a
-- now-canonical posting out of search until the next rebuild, silently.
requeued AS (
    INSERT INTO search_outbox (job_id, job_posted_at)
    SELECT id, eff_posted_at FROM flipped WHERE was_duplicate_of IS NOT NULL AND now_duplicate_of IS NULL
    ON CONFLICT (job_id) DO NOTHING
)
SELECT count(*)::bigint FROM updated;

-- name: GetJobDuplicateOwner :one
-- Resolve the source's ultimate owner, including suppressed sources whose parent
-- is itself suppressed. Match ListJobCopies' depth bound; a cycle/missing chain
-- yields zero, so related-jobs reads can decline to recommend their own posting.
WITH RECURSIVE up AS (
    SELECT a.id, a.duplicate_of, 0 AS depth FROM jobs a WHERE a.id = sqlc.arg(job_id)
    UNION ALL
    SELECT p.id, p.duplicate_of, u.depth + 1
    FROM up u
    JOIN jobs p ON p.id = u.duplicate_of
    WHERE u.depth < 8
)
SELECT COALESCE((SELECT u.id FROM up u WHERE u.duplicate_of IS NULL LIMIT 1), 0)::bigint AS owner_id;

-- name: ListJobCopies :many
-- The open postings the anchor's OWNER represents — the "N openings across cities" list for a
-- collapsed role. Each copy keeps its own location and apply URL, so a seeker picks their city;
-- the owner itself is included (it is one of the openings). Ordered by location.
--
-- Membership is the DUPLICATE CLOSURE, not a shared role_fingerprint, and it is deliberately
-- the same closure DuplicateClosureGeoAll unions geography over. The two must not disagree: a
-- posting whose city the canon claims in search but whose row this list omits is a location a
-- candidate can filter to and then not reach. Grouping by fingerprint could only ever see the
-- exact role pass's clusters, so a fuzzy-suppressed per-city variant — the very thing issue
-- #2225 reported — was never listed.
--
-- The anchor MAY itself be suppressed: a hidden posting stays readable by slug, which is how
-- #2225 was reported, so the walk resolves the anchor UP to its ultimate owner first and lists
-- that owner's closure. Answering with the anchor's own subtree would hand back the fragment
-- its marker happens to point at.
--
-- Cycle safety is NOT structural here, unlike the closure geography queries: those seed from
-- rows that are nobody's duplicate, which makes a cycle unreachable, but this one is handed an
-- arbitrary id. The depth bound on the upward walk is therefore load-bearing — an anchor inside
-- a marker cycle simply resolves to no owner and lists nothing.
--
-- The upward walk does not test closed_at. An anchor pointing at a closed parent still resolves
-- through it to the open owner, so the closed row costs the group nothing; the final filter is
-- what keeps closed rows out of the OUTPUT. That makes this list broader than search in one
-- direction only — it can show an open posting search does not — which is the safe direction:
-- listing a posting a candidate can apply to is never a leak, and hiding one is the complaint.
--
-- AND NOT j.is_private excludes the jd-tailor-intake private-job path: without it, a private
-- job inside the same closure would surface (slug, location, url) to anyone browsing that
-- PUBLIC job's copies — a listing leak, not merely "you'd need the direct link", which is what
-- never indexing/listing it is for.
WITH RECURSIVE up AS (
    SELECT a.id, a.duplicate_of, 0 AS depth
    FROM jobs a
    WHERE a.id = sqlc.arg(job_id)
    UNION ALL
    SELECT p.id, p.duplicate_of, u.depth + 1
    FROM up u
    JOIN jobs p ON p.id = u.duplicate_of
    WHERE u.depth < 8
),
owner AS (
    SELECT id FROM up WHERE duplicate_of IS NULL LIMIT 1
),
member AS (
    SELECT o.id AS member_id, 0 AS depth FROM owner o
    UNION ALL
    SELECT c.id, m.depth + 1
    FROM member m
    JOIN jobs c ON c.duplicate_of = m.member_id
    WHERE c.closed_at IS NULL AND m.depth < 8
)
SELECT j.public_slug, j.location, j.url, j.posted_at,
    COUNT(*) OVER()::bigint AS total
FROM member m
JOIN jobs j ON j.id = m.member_id
WHERE j.closed_at IS NULL
  AND NOT j.is_private
ORDER BY j.location, j.id
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: RoleClusterCountsAll :many
-- The whole-catalogue role-cluster counts in one aggregate pass, for the reindex to
-- build its (company_slug, role_fingerprint) -> counts lookup once. Only clusters with
-- more than one posting are returned (singletons are the count-1 default a lookup miss
-- already implies), keeping the map small. NULL/empty fingerprints are excluded.
SELECT
    company_slug,
    role_fingerprint,
    COUNT(*)::bigint AS repost_count,
    COUNT(*) FILTER (WHERE closed_at IS NULL)::bigint AS mass_count
FROM jobs
WHERE role_fingerprint IS NOT NULL AND role_fingerprint <> ''
GROUP BY company_slug, role_fingerprint
HAVING COUNT(*) > 1;

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

-- name: CompaniesWithRoleClusters :many
-- Company slugs whose role-duplicate markers may need recomputing: a company with an
-- open role cluster (>1 posting sharing a fingerprint) to collapse, OR one still
-- carrying an open marker that may need clearing (its cluster shrank). The recompute
-- processes these ONE COMPANY AT A TIME (RecomputeRoleDuplicatesForCompany) in short
-- transactions, so it never holds a table-wide lock that would stall concurrent ingest
-- crawls (a whole-table UPDATE did: it locked ~1.4M rows for minutes).
SELECT company_slug FROM jobs
WHERE closed_at IS NULL AND company_slug <> '' AND role_fingerprint IS NOT NULL AND role_fingerprint <> ''
GROUP BY company_slug, role_fingerprint
HAVING COUNT(*) > 1
UNION
SELECT DISTINCT company_slug FROM jobs
WHERE closed_at IS NULL AND company_slug <> '' AND duplicate_of IS NOT NULL;

-- name: RecomputeRoleDuplicatesForCompanies :one
-- The batched slice of the role-duplicate recompute, driven over a CHUNK of companies
-- (cmd/reindex's forCompanyBatches) rather than one call per company — see that
-- function's doc comment for why: at catalogue scale (2026-08-06 prod measurement:
-- 236,923 distinct open companies) one round trip per company made this pass take
-- hours under ordinary host load, most of it network/planning overhead rather than
-- query cost. Batching multiple companies into ONE statement is safe here without any
-- extra cross-company guard: role_fingerprint is sha256(company_slug, title,
-- description) (internal/job/jobhash.RoleFingerprint) with company_slug as its FIRST
-- component, so a fingerprint collision between two different companies is not a
-- realistic concern — grouping by role_fingerprint alone, across the whole batch,
-- cannot merge two different companies' rows. Canon = min(id) among a role's open rows;
-- the canon and any singleton/empty-fp row get duplicate_of NULL, the other reposts
-- point to the canon. Rows are never deleted, so the reality counts are untouched. The
-- (company_slug, role_fingerprint) index makes both scans range scans over the batch.
-- The IS DISTINCT FROM guard makes re-runs cheap and idempotent, and a closed canon
-- fails over to the next min(id) on the next run.
-- A private row is never canon, the rule CanonicalJobForRole states at write time: a JD pasted
-- before the employer's board entered the catalogue predates every public posting of the role,
-- and electing it would mark those postings duplicates of a row nothing lists, taking them out
-- of search. It still joins target below, so it can be marked a duplicate of a public canon,
-- which costs nothing to a row that is already invisible.
WITH canon AS (
    SELECT jobs.role_fingerprint, MIN(jobs.id) AS canon_id, COUNT(*) AS n
    FROM jobs
    WHERE jobs.company_slug = ANY(sqlc.arg(companies)::text[])
      AND jobs.closed_at IS NULL AND jobs.role_fingerprint IS NOT NULL AND jobs.role_fingerprint <> ''
      AND NOT jobs.is_private
    GROUP BY jobs.role_fingerprint
),
target AS (
    SELECT j.id,
        CASE WHEN c.n > 1 AND j.id <> c.canon_id THEN c.canon_id END AS new_dup,
        j.duplicate_of AS was_duplicate_of
    FROM jobs j
    JOIN canon c ON j.role_fingerprint = c.role_fingerprint
    WHERE j.company_slug = ANY(sqlc.arg(companies)::text[]) AND j.closed_at IS NULL
),
updated AS (
    UPDATE jobs j
    SET duplicate_of_role = t.new_dup,
        updated_at        = now()
    FROM target t
    WHERE j.id = t.id
      AND j.duplicate_of_role IS DISTINCT FROM t.new_dup
    RETURNING j.id,
              t.was_duplicate_of,
              j.duplicate_of AS now_duplicate_of,
              COALESCE(j.posted_at, j.created_at) AS eff_posted_at
),
dequeued AS (
    INSERT INTO search_delete_outbox (job_id)
    SELECT id FROM updated WHERE was_duplicate_of IS NULL AND now_duplicate_of IS NOT NULL
    ON CONFLICT (job_id) DO NOTHING
),
requeued AS (
    INSERT INTO search_outbox (job_id, job_posted_at)
    SELECT id, eff_posted_at FROM updated WHERE was_duplicate_of IS NOT NULL AND now_duplicate_of IS NULL
    ON CONFLICT (job_id) DO NOTHING
)
SELECT count(*)::bigint FROM updated;
