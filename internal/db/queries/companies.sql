-- name: ListCompanies :many
-- Catalog page: companies with their job counts. The job count is computed on
-- the fly (no denormalized counter yet). This is the one acknowledged place a
-- join to jobs is acceptable; LEFT JOIN keeps companies with zero jobs visible.
-- An empty `search` short-circuits the ILIKE, so the same prepared statement
-- serves both the full list and a name search (`search` is a case-insensitive
-- substring of the name).
SELECT c.slug, c.name, count(j.company_slug) AS job_count
FROM companies c
LEFT JOIN jobs j ON j.company_slug = c.slug AND j.closed_at IS NULL
WHERE sqlc.arg('search')::text = '' OR c.name ILIKE '%' || sqlc.arg('search') || '%'
GROUP BY c.slug, c.name
ORDER BY c.name
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
