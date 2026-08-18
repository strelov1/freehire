-- name: ResolveCompanySlugAliases :many
-- The canonical slug for each folded key in the batch — one round trip per board run, not
-- one per posting. pipeline.Runner folds the run's distinct company slugs, asks this once,
-- and hands the ONE resulting map to both the aggregator-coverage gate and the upsert, so
-- neither can compare a slug the other would not have written.
--
-- Keyed on folded_key rather than on alias_slug so a spelling that was never itself merged
-- still lands: "DollarTree" has no row, but it folds onto "dollartree", which does.
--
-- Ordered so the answer is deterministic. One canonical slug per folded key is an invariant
-- the WRITER holds — planMerges elects exactly one winner per folded group and never
-- re-elects against a frozen canon — but the schema cannot express "at most one DISTINCT
-- canonical_slug per folded_key" without a second table, and this design deliberately keeps
-- one. Without the ORDER BY a violation would resolve differently run to run; with it, the
-- adapter can also see the conflict and say so.
SELECT folded_key, canonical_slug
FROM company_slug_aliases
WHERE folded_key = ANY(@folded_keys::text[])
ORDER BY folded_key, canonical_slug;

-- name: GetCompanySlugAlias :one
-- Where a retired slug should 301 to. GET /companies/:slug consults this only AFTER missing
-- in `companies`, so a re-created company always wins over a stale alias.
SELECT canonical_slug
FROM company_slug_aliases
WHERE alias_slug = @alias_slug;

-- name: InsertCompanySlugAlias :execrows
-- Record a merge. DO NOTHING on conflict because the canon is frozen at first merge: a later
-- run that re-elects a different winner for the same folded group must not silently move a
-- URL that has already been 301-ing and indexed.
INSERT INTO company_slug_aliases (alias_slug, canonical_slug, folded_key, reason)
VALUES (@alias_slug, @canonical_slug, @folded_key, @reason)
ON CONFLICT (alias_slug) DO NOTHING;

-- name: ListCanonicalCompanySlugs :many
-- Every slug already elected canonical. The merge worker holds these out of a new election,
-- which is what "frozen" means in practice.
SELECT DISTINCT canonical_slug FROM company_slug_aliases;

-- name: ListCompaniesForMerge :many
-- The merge worker's input: every company slug the JOBS table actually holds, with the display
-- name and the open-job count that elects the winner of its folded group.
--
-- READ FROM jobs, NOT FROM companies.job_count. That column counts the postings the SEARCH
-- INDEX holds, not the rows this worker rewrites, and the two diverge exactly where it hurts:
-- a slug a merge has already retired drops to 0 in the index while its unmoved rows stay in
-- the table. Planning from the counter made those rows INVISIBLE to the planner — 8,375 of
-- them on `jpmorganchase` alone, stranded permanently, because the better the merge worked the
-- more reliably the remainder hid.
--
-- The name is the most recent one seen for the slug, matching how SyncCompaniesFromJobs
-- collapses a slug's name variants. Grouping into folded groups happens in Go because the fold
-- is normalize.CompanyKey — a repeating legal-form strip no SQL expression should reproduce,
-- since a second implementation of that rule is the bug this whole change removes.
SELECT company_slug AS slug,
       ((array_agg(company ORDER BY created_at DESC))[1])::text AS name,
       count(*) FILTER (WHERE closed_at IS NULL)::int AS job_count
FROM jobs
WHERE company_slug <> ''
GROUP BY company_slug;

-- name: RekeyCompanySlugChunk :execrows
-- Move one chunk of a retired slug's jobs onto the canonical slug.
--
-- company_slug_folded is written in the same statement, as every write path that sets
-- company_slug must (migration 0109; internal/db/folded_slug_rule_test.go enforces it).
--
-- Idempotent by construction rather than by a guard: the subquery selects rows still
-- carrying the OLD slug, so an updated row leaves the set. A re-run updates zero, and
-- stopping mid-way costs nothing — the next run resumes with what is left.
UPDATE jobs
SET company_slug = @canonical_slug,
    company_slug_folded = replace(@canonical_slug, '-', ''),
    updated_at = now()
WHERE jobs.id IN (
    SELECT j.id
    FROM jobs j
    WHERE j.company_slug = @alias_slug
    ORDER BY j.id
    LIMIT @chunk_size
);

-- name: ListCompanySlugAliases :many
-- The whole registry, folded key to canonical slug. cmd/backfill-derive loads it once per run
-- and resolves in memory: it re-derives every job in the table, and jobderive is pure, so
-- without this a backfill would silently move every merged posting back to the spelling its
-- source happened to use — undoing the merges and taking role_fingerprint, which is computed
-- from the company slug, with it.
--
-- Ordered for the same reason ResolveCompanySlugAliases is: one canonical slug per folded key
-- is the writer's invariant, and a violation should resolve identically every run.
SELECT DISTINCT folded_key, canonical_slug
FROM company_slug_aliases
ORDER BY folded_key, canonical_slug;
