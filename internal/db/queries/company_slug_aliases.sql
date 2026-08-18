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
-- The merge worker's input: every company that has an open job, with the count that elects
-- the winner of its folded group. Grouping happens in Go because the fold is
-- normalize.CompanyKey — a repeating legal-form strip no SQL expression should try to
-- reproduce, since a second implementation of that rule is the bug this change removes.
SELECT slug, name, job_count
FROM companies
WHERE job_count > 0;

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
