-- name: ListCompanies :many
-- Catalog page: companies with their job counts, most active first. The job count
-- is read from the denormalized companies.job_count column (maintained by
-- cmd/recount-companies), so this read does not join jobs. Ordered by job_count
-- DESC, name — the same ordering the sidebar company typeahead consumes. An empty
-- `search` short-circuits the ILIKE, so the same prepared statement serves both
-- the full list and a name search (`search` is a case-insensitive substring of the
-- name).
SELECT slug, name, job_count
FROM companies
WHERE sqlc.arg('search')::text = '' OR name ILIKE '%' || sqlc.arg('search') || '%'
ORDER BY job_count DESC, name
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountCompanies :one
-- Total companies matching the same optional name filter as ListCompanies, so
-- search pagination reports the filtered total.
SELECT count(*)
FROM companies
WHERE sqlc.arg('search')::text = '' OR name ILIKE '%' || sqlc.arg('search') || '%';

-- name: GetCompany :one
-- SELECT * (not an explicit column list) so the generated row stays db.Company as
-- the table grows columns (e.g. collections); an explicit subset makes sqlc emit a
-- distinct row type and breaks the company-detail handler on every new column.
SELECT *
FROM companies
WHERE slug = $1;

-- name: ListCompanyCollections :many
-- All companies with their current collection membership. cmd/import-collections
-- reads this to know the existing company slugs (the match target) and each
-- company's current tags (so it can reconcile only the tags it manages, leaving any
-- others untouched).
SELECT slug, collections
FROM companies
ORDER BY slug;

-- name: SetCompanyCollections :exec
-- Replace a company's collection set. The import worker computes the full set in Go
-- (preserving unmanaged tags) and writes it here; updated_at is bumped for parity
-- with the other write paths.
UPDATE companies
SET collections = $2,
    updated_at  = now()
WHERE slug = $1;

-- name: SyncCompaniesFromJobs :exec
-- Rebuild the companies catalogue from jobs. The companies table is derivable
-- from jobs (slug = company_slug, name = company), so after a slug-builder change
-- re-keys jobs, this re-keys companies to match. DISTINCT ON collapses a slug's
-- name variants; ON CONFLICT folds collisions and refreshes existing rows.
INSERT INTO companies (slug, name)
SELECT DISTINCT ON (company_slug) company_slug, company
FROM jobs
WHERE company_slug <> ''
ORDER BY company_slug
ON CONFLICT (slug) DO UPDATE SET
    name       = EXCLUDED.name,
    updated_at = now();

-- name: DeleteOrphanCompanies :execrows
-- Drop companies no longer referenced by any job — the stale rows left behind
-- when a slug-builder change re-keys jobs onto new slugs.
DELETE FROM companies c
WHERE NOT EXISTS (SELECT 1 FROM jobs j WHERE j.company_slug = c.slug);

-- name: RecountCompanyJobCounts :execrows
-- Recompute every company's denormalized open-job count in one set-based pass:
-- aggregate open jobs (closed_at IS NULL) once by company_slug, LEFT JOIN it onto
-- companies so a company with no open jobs is zeroed (COALESCE), and write the
-- result. The `IS DISTINCT FROM` guard skips rows whose count is already correct,
-- so re-running rewrites nothing and the affected-rows count reports real churn.
-- This is cmd/recount-companies' whole job; run periodically (eventual consistency).
UPDATE companies c
SET job_count = COALESCE(o.cnt, 0)
FROM companies c2
LEFT JOIN (
    SELECT company_slug, count(*) AS cnt
    FROM jobs
    WHERE closed_at IS NULL AND company_slug <> ''
    GROUP BY company_slug
) o ON o.company_slug = c2.slug
WHERE c.slug = c2.slug
  AND c.job_count IS DISTINCT FROM COALESCE(o.cnt, 0);
