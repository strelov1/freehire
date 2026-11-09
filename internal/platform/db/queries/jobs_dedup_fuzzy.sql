-- name: RoleClusterCountsFor :many
-- Role-cluster counts for a SPECIFIC set of (company_slug, role_fingerprint) pairs, so
-- a read path can resolve a page of cards in one query instead of one per card.
--
-- RoleClusterCountsAll exists beside this and is not a substitute: it aggregates the
-- whole catalogue, which is right for a reindex building its lookup once and ruinous
-- for a request. Filtering on role_fingerprint alone would not do either, since
-- jobs_company_role_fingerprint_idx leads with company_slug; leading with the company
-- set keeps the index usable.
--
-- The two sets are matched as a cross product here and narrowed to the exact pairs by
-- the caller. A pair-wise join would need a two-argument unnest the query analyzer
-- cannot type, and the surplus is bounded: a page holds few distinct companies, and a
-- fingerprint belonging to another of them simply has no rows.
SELECT j.company_slug,
       j.role_fingerprint,
       COUNT(*)::bigint AS repost_count,
       COUNT(*) FILTER (WHERE j.closed_at IS NULL)::bigint AS mass_count
FROM jobs j
WHERE j.company_slug = ANY(sqlc.arg(company_slugs)::text[])
  AND j.role_fingerprint = ANY(sqlc.arg(role_fingerprints)::text[])
GROUP BY j.company_slug, j.role_fingerprint;

-- name: CompaniesWithFuzzyDedupCandidates :many
-- Company slugs worth running the fuzzy-description pass over: a company that still has more
-- than one open CANONICAL posting after the exact role-cluster and aggregator passes, so there
-- is something left that byte-exact matching did not collapse. Rows without a company_slug are
-- excluded: the pass buckets by (company, title) and relies on that bucket to keep unrelated
-- roles apart, and an empty slug is not a company boundary — measured on prod, 105 212 such rows
-- fall into 20 126 same-title buckets spanning up to four different employers.
-- The pass then processes these ONE COMPANY AT A TIME, like the other duplicate passes, so it
-- never holds a lock wide enough to stall a concurrent ingest crawl.
--
-- The predicate names the OWNED columns of the exact passes, not the derived duplicate_of. Those
-- are not the same set: duplicate_of also carries this pass's OWN marker, so filtering it made
-- the pass blind to its own output and every marker permanent. And the worklist is a UNION,
-- because a company can need a RELEASE without needing a collapse — its second candidate may
-- have closed since. A collapse-only worklist never visits it again, which is how 42 633 open
-- duplicates (prod, 2026-09-01) came to sit behind a closed owner with no pass that would ever
-- let them go. This mirrors CompaniesWithRoleClusters, which unions the same two reasons.
SELECT company_slug
FROM jobs
WHERE closed_at IS NULL AND company_slug <> ''
  AND duplicate_of_aggregator IS NULL AND duplicate_of_role IS NULL
GROUP BY company_slug
HAVING COUNT(*) > 1
UNION
SELECT DISTINCT company_slug
FROM jobs
WHERE closed_at IS NULL AND company_slug <> '' AND duplicate_of_fuzzy IS NOT NULL;

-- name: FuzzyDedupCandidateTitlesForCompany :many
-- The (id, title) of one company's open canonical postings — deliberately WITHOUT the
-- description. The caller groups these into buckets with the same normalized-title function the
-- rest of the codebase uses (jobhash.RoleKey), then loads descriptions for the buckets that
-- survive the size filter via GetJobDescriptionsByIDs. Normalizing here in SQL instead would
-- duplicate that logic in a second language and let the two drift apart; titles are cheap to
-- ship, descriptions are not.
--
-- "Canonical" here means NOT CLAIMED BY AN EXACT PASS, which is why the predicate names their
-- two owned columns rather than the derived duplicate_of. A row carrying this pass's OWN marker
-- is offered again on purpose: it is the only way the pass can ever re-decide it, and without
-- that a marker survives its canon closing, the descriptions diverging, and every later run.
SELECT id, title
FROM jobs
WHERE company_slug = sqlc.arg(company) AND closed_at IS NULL
  AND duplicate_of_aggregator IS NULL AND duplicate_of_role IS NULL
ORDER BY id;

-- name: MarkFuzzyDuplicatesForCompany :one
-- Write one company's whole fuzzy verdict in a single statement rather than a round trip per
-- row: `candidates` is every row the pass CONSIDERED, and (ids, canons) are the ones it
-- clustered. A candidate with no assignment gets NULL — that is the release, and it is the only
-- mechanism there is.
--
-- It has to be, and the comment this replaces said otherwise for a year. Migrations 0114/0115
-- moved the marker into duplicate_of_fuzzy and left duplicate_of to a trigger deriving it from
-- the three owned columns, so the role recompute — which writes duplicate_of_role — can no
-- longer reach this one, and the COALESCE keeps surfacing it. Nothing else releases a fuzzy
-- marker. Measured on prod 2026-09-01: 42 633 open duplicates behind a closed owner, invisible
-- in search, with no pass that would ever let them go.
--
-- `candidates` is deliberately NOT "every open row of the company". It is what the pass looked
-- at, which excludes the buckets it refuses to judge (over the size cap). A cap is a cost
-- heuristic, not a verdict, so releasing what it skipped would silently un-collapse the largest
-- groups in the catalogue. A row in a bucket too SMALL to cluster is a different thing — the
-- pass did judge it, and the answer was "no cluster".
--
-- Scoped to the company and to open rows, so a row the exact pass claimed in the meantime is
-- left alone. The IS DISTINCT FROM guard makes a re-run free.
WITH candidate AS (
    SELECT unnest(sqlc.arg(candidates)::bigint[]) AS id
),
assign AS (
    SELECT unnest(sqlc.arg(ids)::bigint[]) AS id,
           unnest(sqlc.arg(canons)::bigint[]) AS canon_id
),
target AS (
    -- LEFT JOIN is what turns "considered but not clustered" into NULL. Mirrors the CASE in
    -- RecomputeRoleDuplicatesForCompanies, which is how the role pass releases its own.
    SELECT c.id, a.canon_id AS new_dup
    FROM candidate c
    LEFT JOIN assign a ON a.id = c.id
),
before AS (
    -- Every CTE of one statement reads the same snapshot, so this is the derived marker as it
    -- stood BEFORE the update below — which is what makes the status transition readable
    -- without a second statement and the race between them.
    SELECT j.id, j.duplicate_of AS was_duplicate_of
    FROM jobs j JOIN target t ON t.id = j.id
),
updated AS (
    UPDATE jobs j
    SET duplicate_of_fuzzy = m.new_dup,
        updated_at         = now()
    FROM target m
    WHERE j.id = m.id
      AND j.company_slug = sqlc.arg(company)
      AND j.closed_at IS NULL
      AND j.duplicate_of_fuzzy IS DISTINCT FROM m.new_dup
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
requeued AS (
    INSERT INTO search_outbox (job_id, job_posted_at)
    SELECT id, eff_posted_at FROM flipped WHERE was_duplicate_of IS NOT NULL AND now_duplicate_of IS NULL
    ON CONFLICT (job_id) DO NOTHING
)
SELECT count(*)::bigint FROM flipped;

-- name: OrphanAggregatorCompanies :many
-- Companies the catalogue holds ONLY through aggregators — the worklist cmd/harvest-orphans
-- turns into candidate ATS boards. A company qualifies when it has an open posting from one
-- of the REQUESTED aggregators and no open posting from any source outside the FULL
-- aggregator set.
--
-- The two provider sets are deliberately separate. Narrowing a run to one aggregator must
-- not make another aggregator's posting look like first-party ATS coverage: the candidate
-- scan uses `requested`, the exclusion test always uses `aggregators`. Auditing this same
-- distinction with a partial list is what inflated the July aggregator-dedup leak count
-- roughly fourfold.
--
-- The display name is the modal `company` across the aggregator rows, since two aggregators
-- may spell the same employer differently and the name is what the harvest gate compares.
--
-- NOT ats.is_private sits on the EXCLUSION test, which is where it can change an answer:
-- a private posting's source is 'pasted'/'weblink' (internal/job/privatejob), so it is
-- never a candidate row above, but it does satisfy "a source outside the aggregator set"
-- and would silently disqualify a company nobody's board is actually crawled for. This is
-- the argument CompaniesWithFreshNonAggregatorCoverage above already writes out in full:
-- a JD one user pasted in is not evidence that we cover an employer.
SELECT j.company_slug,
       (mode() WITHIN GROUP (ORDER BY j.company))::text AS company
FROM jobs j
WHERE j.closed_at IS NULL
  AND j.company_slug <> ''
  AND j.source = ANY(sqlc.arg(requested)::text[])
  AND NOT EXISTS (
    SELECT 1 FROM jobs ats
    WHERE ats.company_slug = j.company_slug
      AND ats.closed_at IS NULL
      AND NOT ats.is_private
      AND ats.source <> ALL(sqlc.arg(aggregators)::text[])
  )
GROUP BY j.company_slug
ORDER BY count(*) DESC, j.company_slug;
