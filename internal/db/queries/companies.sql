-- name: ListCompanies :many
-- Catalog page: companies with their job counts, most active first. The job count
-- is read from the denormalized companies.job_count column (maintained by
-- cmd/recount-companies), so this read does not join jobs. Ordered by job_count
-- DESC, name — the same ordering the sidebar company typeahead consumes. An empty
-- `search` short-circuits the ILIKE, so the same prepared statement serves both
-- the full list and a name search (`search` is a case-insensitive substring of the
-- name). Each facet param is a text[] filtered by array overlap (&&): an empty
-- array short-circuits to no constraint, non-empty values are OR-ed within the
-- facet, and the facets AND together (and with the name search). `remote_regions`
-- is the job-derived facet scoped to remote jobs (see RefreshCompanyFacets), a
-- subset of `regions`. The name search also matches the slug, so a hyphenated slug
-- query ("ge-vernova") finds the company even though its name has a space ("GE
-- Vernova"). CountCompanies MUST keep an identical WHERE so the filtered total
-- matches the page. `job_count > 0` scopes the catalog to companies that are
-- actually hiring, excluding the ~92k job-less reference rows imported by the YC
-- and company-info backfills; it also lets both reads ride companies_hiring_job_count_idx
-- (partial index) instead of scanning the full 2.3 GB heap.
SELECT slug, name, job_count, tagline, industries, hq_country
FROM companies
WHERE job_count > 0
  AND (sqlc.arg('search')::text = '' OR name ILIKE '%' || sqlc.arg('search') || '%' OR slug ILIKE '%' || sqlc.arg('search') || '%')
  AND (coalesce(cardinality(sqlc.arg('collections')::text[]), 0) = 0 OR collections && sqlc.arg('collections')::text[])
  AND (coalesce(cardinality(sqlc.arg('regions')::text[]), 0) = 0 OR regions && sqlc.arg('regions')::text[])
  AND (coalesce(cardinality(sqlc.arg('countries')::text[]), 0) = 0 OR countries && sqlc.arg('countries')::text[])
  AND (coalesce(cardinality(sqlc.arg('domains')::text[]), 0) = 0 OR domains && sqlc.arg('domains')::text[])
  AND (coalesce(cardinality(sqlc.arg('company_types')::text[]), 0) = 0 OR company_types && sqlc.arg('company_types')::text[])
  AND (coalesce(cardinality(sqlc.arg('company_sizes')::text[]), 0) = 0 OR company_sizes && sqlc.arg('company_sizes')::text[])
  AND (coalesce(cardinality(sqlc.arg('remote_regions')::text[]), 0) = 0 OR remote_regions && sqlc.arg('remote_regions')::text[])
  AND (coalesce(cardinality(sqlc.arg('yc_batch')::text[]), 0) = 0 OR yc_batch && sqlc.arg('yc_batch')::text[])
  AND (coalesce(cardinality(sqlc.arg('yc_status')::text[]), 0) = 0 OR yc_status && sqlc.arg('yc_status')::text[])
  AND (coalesce(cardinality(sqlc.arg('yc_stage')::text[]), 0) = 0 OR yc_stage && sqlc.arg('yc_stage')::text[])
  AND (coalesce(cardinality(sqlc.arg('yc_flags')::text[]), 0) = 0 OR yc_flags && sqlc.arg('yc_flags')::text[])
  -- maturity is a SCALAR column (not an array): membership, not overlap. A NULL
  -- (unknown) maturity matches no requested value, so `NULL = ANY(...)` excludes it.
  AND (coalesce(cardinality(sqlc.arg('maturity')::text[]), 0) = 0 OR maturity = ANY(sqlc.arg('maturity')::text[]))
  -- subindustry is likewise a NULLABLE SCALAR: membership, not overlap; NULL matches none.
  AND (coalesce(cardinality(sqlc.arg('subindustries')::text[]), 0) = 0 OR subindustry = ANY(sqlc.arg('subindustries')::text[]))
ORDER BY job_count DESC, name
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountCompanies :one
-- Total companies matching the same optional name + facet filters as ListCompanies,
-- so search/filter pagination reports the filtered total. Keep this WHERE identical
-- to ListCompanies (including the job_count > 0 hiring scope).
SELECT count(*)
FROM companies
WHERE job_count > 0
  AND (sqlc.arg('search')::text = '' OR name ILIKE '%' || sqlc.arg('search') || '%' OR slug ILIKE '%' || sqlc.arg('search') || '%')
  AND (coalesce(cardinality(sqlc.arg('collections')::text[]), 0) = 0 OR collections && sqlc.arg('collections')::text[])
  AND (coalesce(cardinality(sqlc.arg('regions')::text[]), 0) = 0 OR regions && sqlc.arg('regions')::text[])
  AND (coalesce(cardinality(sqlc.arg('countries')::text[]), 0) = 0 OR countries && sqlc.arg('countries')::text[])
  AND (coalesce(cardinality(sqlc.arg('domains')::text[]), 0) = 0 OR domains && sqlc.arg('domains')::text[])
  AND (coalesce(cardinality(sqlc.arg('company_types')::text[]), 0) = 0 OR company_types && sqlc.arg('company_types')::text[])
  AND (coalesce(cardinality(sqlc.arg('company_sizes')::text[]), 0) = 0 OR company_sizes && sqlc.arg('company_sizes')::text[])
  AND (coalesce(cardinality(sqlc.arg('remote_regions')::text[]), 0) = 0 OR remote_regions && sqlc.arg('remote_regions')::text[])
  AND (coalesce(cardinality(sqlc.arg('yc_batch')::text[]), 0) = 0 OR yc_batch && sqlc.arg('yc_batch')::text[])
  AND (coalesce(cardinality(sqlc.arg('yc_status')::text[]), 0) = 0 OR yc_status && sqlc.arg('yc_status')::text[])
  AND (coalesce(cardinality(sqlc.arg('yc_stage')::text[]), 0) = 0 OR yc_stage && sqlc.arg('yc_stage')::text[])
  AND (coalesce(cardinality(sqlc.arg('yc_flags')::text[]), 0) = 0 OR yc_flags && sqlc.arg('yc_flags')::text[])
  AND (coalesce(cardinality(sqlc.arg('maturity')::text[]), 0) = 0 OR maturity = ANY(sqlc.arg('maturity')::text[]))
  AND (coalesce(cardinality(sqlc.arg('subindustries')::text[]), 0) = 0 OR subindustry = ANY(sqlc.arg('subindustries')::text[]));

-- name: EstimateHiringCompanies :one
-- Fast approximate hiring-company total (job_count > 0) for the UNFILTERED /companies
-- list's meta.total. An exact count(*) over the ~227k hiring rows is a cold-cache heap
-- scan (~17s on prod, see migration 0034); the planner's estimate is O(1). Only the
-- no-filter catalogue count uses this — every facet/search filter narrows to an index
-- and keeps CountCompanies cheap and exact. Approximate by design, like EstimateOpenJobs.
SELECT estimate_hiring_companies()::bigint;

-- name: CompanySubindustries :many
-- Distinct non-NULL subindustry values with their company counts, most common first
-- (ties broken by value), serving the searchable option list for the subindustry facet.
-- Counts are unconditional — they do not reflect other active list filters.
SELECT subindustry AS value, count(*) AS count
FROM companies
WHERE subindustry IS NOT NULL
GROUP BY subindustry
ORDER BY count(*) DESC, subindustry;

-- name: ListCompanySitemap :many
-- Slim keyset page of companies for the sitemap, cursored by the slug primary key
-- (first chunk keyed by the empty string, which sorts before every slug).
SELECT slug, updated_at
FROM companies
WHERE slug > sqlc.arg(after_slug)
ORDER BY slug
LIMIT sqlc.arg(batch_size);

-- name: CompanySitemapBoundaries :many
-- The slug ending every full chunk of `chunk_size` companies (ordered by slug),
-- excluding the final row, so the sitemap index can list each company sub-sitemap's
-- keyset cursor.
SELECT slug FROM (
  SELECT slug,
         row_number() OVER (ORDER BY slug) AS rn,
         count(*) OVER () AS total
  FROM companies
) t
WHERE rn % sqlc.arg(chunk_size)::bigint = 0 AND rn < total
ORDER BY slug;

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

-- name: ListSlugLikeCompaniesForBackfill :many
-- Companies whose ingested name is still a squished slug (lowercase, no
-- whitespace or uppercase) and that have at least one open job, with a
-- representative open job's source and URL so the backfill worker can locate the
-- ATS board. Only boards with live jobs matter, so dead ones never appear. The Go
-- side re-validates slug-likeness authoritatively before touching anything.
SELECT DISTINCT ON (company_slug)
       company_slug AS slug,
       company      AS name,
       source,
       url
FROM jobs
WHERE closed_at IS NULL
  AND duplicate_of IS NULL
  AND company_slug <> ''
  AND company ~ '^[a-z0-9._-]+$'
ORDER BY company_slug, created_at DESC;

-- name: RenameSlugCompany :execrows
-- Apply a resolved display name to every job under a slug-like company and
-- re-key its company_slug (computed by the caller via normalize.Slug), so the
-- derived catalogue re-keys through SyncCompaniesFromJobs + DeleteOrphanCompanies.
-- The name guard keeps a re-run from overwriting a name that is no longer a slug.
UPDATE jobs
SET company = @name, company_slug = @new_slug, updated_at = now()
WHERE company_slug = @old_slug
  AND company ~ '^[a-z0-9._-]+$';

-- name: DeleteOrphanCompanies :execrows
-- Drop companies no longer referenced by any job — the stale rows left behind
-- when a slug-builder change re-keys jobs onto new slugs. Reference rows imported
-- by the company-info backfill are preserved: they intentionally have no job, so
-- the NOT is_reference guard keeps the backfill directory from being swept away.
DELETE FROM companies c
WHERE NOT c.is_reference
  AND NOT EXISTS (SELECT 1 FROM jobs j WHERE j.company_slug = c.slug);

-- name: CompanyExists :one
-- Whether a company row already exists for the slug. The backfill checks this
-- before upserting to log matched-existing vs inserted-reference counts — the
-- upsert itself is blind to which path (insert or update) it took.
SELECT EXISTS(SELECT 1 FROM companies WHERE slug = $1);

-- name: UpsertCompanyInfo :exec
-- Apply one external-dataset company-info record, matched by slug. A new slug is
-- inserted as a reference row (is_reference = true) with no jobs; an existing slug
-- (job-backed or a prior reference) has only its company-info columns refreshed —
-- name, job_count, collections, is_reference, and the job-derived facet arrays are
-- left untouched. Idempotent: re-running the same record rewrites the same values.
INSERT INTO companies (
    slug, name, industries, year_founded, employee_count, hq_country,
    organization_type, tagline, company_info, is_reference, company_info_at
) VALUES (
    sqlc.arg(slug), sqlc.arg(name), sqlc.arg(industries), sqlc.arg(year_founded),
    sqlc.arg(employee_count), sqlc.arg(hq_country), sqlc.arg(organization_type),
    sqlc.arg(tagline), sqlc.arg(company_info), true, now()
)
ON CONFLICT (slug) DO UPDATE SET
    industries        = EXCLUDED.industries,
    year_founded      = EXCLUDED.year_founded,
    employee_count    = EXCLUDED.employee_count,
    hq_country        = EXCLUDED.hq_country,
    organization_type = EXCLUDED.organization_type,
    tagline           = EXCLUDED.tagline,
    company_info      = EXCLUDED.company_info,
    company_info_at   = now(),
    updated_at        = now();

-- name: UpsertYCCompany :exec
-- Apply one yc-oss directory entry, matched by slug. A new slug is inserted as a
-- reference row (is_reference = true) with no jobs; an existing slug (job-backed or a
-- prior reference) has its company-info columns plus the curated yc_batch/yc_status
-- facets refreshed — name, job_count, collections, is_reference, and the job-derived
-- facet arrays (regions/remote_regions/countries/domains/company_types/company_sizes)
-- are left untouched. Idempotent: re-running the same entry rewrites the same values.
INSERT INTO companies (
    slug, name, industries, subindustry, year_founded, employee_count, hq_country,
    tagline, company_info, yc_batch, yc_status, yc_stage, yc_flags,
    is_reference, company_info_at
) VALUES (
    sqlc.arg(slug), sqlc.arg(name), sqlc.arg(industries), sqlc.arg(subindustry),
    sqlc.arg(year_founded), sqlc.arg(employee_count), sqlc.arg(hq_country), sqlc.arg(tagline),
    sqlc.arg(company_info), sqlc.arg(yc_batch), sqlc.arg(yc_status),
    sqlc.arg(yc_stage), sqlc.arg(yc_flags), true, now()
)
ON CONFLICT (slug) DO UPDATE SET
    industries      = EXCLUDED.industries,
    subindustry     = EXCLUDED.subindustry,
    year_founded    = EXCLUDED.year_founded,
    employee_count  = EXCLUDED.employee_count,
    hq_country      = EXCLUDED.hq_country,
    tagline         = EXCLUDED.tagline,
    company_info    = EXCLUDED.company_info,
    yc_batch        = EXCLUDED.yc_batch,
    yc_status       = EXCLUDED.yc_status,
    yc_stage        = EXCLUDED.yc_stage,
    yc_flags        = EXCLUDED.yc_flags,
    company_info_at = now(),
    updated_at      = now();

-- name: RefreshCompanyFacets :execrows
-- Recompute every company's denormalized state in one set-based pass: the open-job
-- count plus the facet arrays derived from those open jobs — regions/countries from
-- the jobs geography columns, remote_regions from those same regions but scoped to
-- remote jobs (work_mode='remote'), and domains/company_types/company_sizes from the
-- jobs.enrichment JSONB. Each array is the distinct union across the company's open
-- jobs (closed_at IS NULL), aggregated with a stable ORDER BY so the guard below
-- compares deterministically. A company with no open jobs (or no remote/enriched
-- jobs) is zeroed/emptied via COALESCE. The per-column `IS DISTINCT FROM` guard skips
-- rows already current, so re-running rewrites nothing and the affected-rows count
-- reports real churn. This is cmd/recount-companies' whole job; run periodically
-- (eventual consistency). The facet aggregates are each their own non-correlated
-- GROUP BY so the row-multiplying unnest of one array never distorts another's count.
WITH oj AS (
    -- duplicate_of IS NULL counts one canonical job per role cluster, so the company
    -- job_count matches the collapsed /jobs and company lists (reposts share facets, so
    -- the DISTINCT region/country aggregates are unaffected — only the count changes).
    SELECT company_slug, regions, countries, enrichment, work_mode, source
    FROM jobs
    WHERE closed_at IS NULL AND duplicate_of IS NULL AND company_slug <> ''
),
counts AS (
    SELECT company_slug, count(*) AS cnt FROM oj GROUP BY company_slug
),
reg AS (
    SELECT company_slug, array_agg(DISTINCT r ORDER BY r) AS arr
    FROM oj CROSS JOIN LATERAL unnest(oj.regions) AS r
    GROUP BY company_slug
),
remote_reg AS (
    SELECT company_slug, array_agg(DISTINCT r ORDER BY r) AS arr
    FROM oj CROSS JOIN LATERAL unnest(oj.regions) AS r
    WHERE oj.work_mode = 'remote'
    GROUP BY company_slug
),
cty AS (
    SELECT company_slug, array_agg(DISTINCT c ORDER BY c) AS arr
    FROM oj CROSS JOIN LATERAL unnest(oj.countries) AS c
    GROUP BY company_slug
),
dom AS (
    SELECT company_slug, array_agg(DISTINCT d ORDER BY d) AS arr
    FROM oj CROSS JOIN LATERAL jsonb_array_elements_text(
        CASE WHEN jsonb_typeof(oj.enrichment->'domains') = 'array'
             THEN oj.enrichment->'domains' ELSE '[]'::jsonb END) AS d
    GROUP BY company_slug
),
ctype AS (
    SELECT company_slug,
           array_agg(DISTINCT (enrichment->>'company_type') ORDER BY (enrichment->>'company_type')) AS arr
    FROM oj
    WHERE COALESCE(enrichment->>'company_type', '') <> ''
    GROUP BY company_slug
),
csize AS (
    SELECT company_slug,
           array_agg(DISTINCT (enrichment->>'company_size') ORDER BY (enrichment->>'company_size')) AS arr
    FROM oj
    WHERE COALESCE(enrichment->>'company_size', '') <> ''
    GROUP BY company_slug
),
-- gov marks a company whose open jobs come from an exclusively-government source
-- (usajobs = US federal, neogov = US state/local gov ATS). Generic ATS (workday,
-- greenhouse, …) carry government jobs too, so they are deliberately NOT a signal.
gov AS (
    SELECT company_slug, bool_or(source IN ('usajobs', 'neogov')) AS is_gov
    FROM oj
    GROUP BY company_slug
),
-- mat is the deterministic single-valued maturity, computed per company from its own
-- signals plus the gov-source marker, in precedence order (government beats size).
-- NULL = unknown (an honest abstain when no signal fits). Computed once here so both
-- the SET and the IS DISTINCT FROM guard reference the same value.
mat AS (
    SELECT co.slug AS company_slug,
           CASE
               WHEN COALESCE(g.is_gov, false) OR co.organization_type = 'Government' THEN 'government'
               -- enterprise beats startup: a grown company is enterprise regardless of a
               -- historical YC badge (YC alumni go Public/Acquired and scale to thousands).
               WHEN co.employee_count >= 1000 THEN 'enterprise'
               -- startup only for a still-ACTIVE YC company (not Public/Acquired/Inactive),
               -- or an independently small-and-recent company.
               WHEN co.yc_status && ARRAY['Active']
                    OR (co.year_founded >= extract(year FROM now())::int - 7 AND co.employee_count <= 50) THEN 'startup'
               WHEN co.employee_count BETWEEN 51 AND 999 THEN 'scaleup'
               ELSE NULL
           END AS val
    FROM companies co
    LEFT JOIN gov g ON g.company_slug = co.slug
),
-- csize_final is the employee_count-authoritative company_sizes hybrid: the company's
-- own recorded headcount (bucketed into the company_size vocabulary) is a single, more
-- accurate value than the LLM's per-posting guess, so it wins when present; otherwise
-- fall back to the distinct union of enrichment.company_size over open jobs (the csize
-- CTE). Computed once so the SET and the IS DISTINCT FROM guard share one value.
csize_final AS (
    SELECT co.slug AS company_slug,
           CASE
               WHEN co.employee_count IS NULL   THEN COALESCE(cs.arr, '{}')
               WHEN co.employee_count <= 10     THEN ARRAY['1-10']
               WHEN co.employee_count <= 50     THEN ARRAY['11-50']
               WHEN co.employee_count <= 200    THEN ARRAY['51-200']
               WHEN co.employee_count <= 500    THEN ARRAY['201-500']
               WHEN co.employee_count <= 1000   THEN ARRAY['501-1000']
               ELSE ARRAY['1000+']
           END AS arr
    FROM companies co
    LEFT JOIN csize cs ON cs.company_slug = co.slug
)
UPDATE companies c
SET job_count      = COALESCE(counts.cnt, 0),
    regions        = COALESCE(reg.arr, '{}'),
    remote_regions = COALESCE(remote_reg.arr, '{}'),
    countries      = COALESCE(cty.arr, '{}'),
    domains        = COALESCE(dom.arr, '{}'),
    company_types  = COALESCE(ctype.arr, '{}'),
    company_sizes  = csize_final.arr,
    maturity       = mat.val
FROM companies c2
LEFT JOIN counts      ON counts.company_slug     = c2.slug
LEFT JOIN reg         ON reg.company_slug        = c2.slug
LEFT JOIN remote_reg  ON remote_reg.company_slug = c2.slug
LEFT JOIN cty         ON cty.company_slug        = c2.slug
LEFT JOIN dom         ON dom.company_slug        = c2.slug
LEFT JOIN ctype       ON ctype.company_slug      = c2.slug
LEFT JOIN csize_final ON csize_final.company_slug = c2.slug
LEFT JOIN mat         ON mat.company_slug        = c2.slug
WHERE c.slug = c2.slug
  AND (c.job_count      IS DISTINCT FROM COALESCE(counts.cnt, 0)
    OR c.regions        IS DISTINCT FROM COALESCE(reg.arr, '{}')
    OR c.remote_regions IS DISTINCT FROM COALESCE(remote_reg.arr, '{}')
    OR c.countries      IS DISTINCT FROM COALESCE(cty.arr, '{}')
    OR c.domains        IS DISTINCT FROM COALESCE(dom.arr, '{}')
    OR c.company_types  IS DISTINCT FROM COALESCE(ctype.arr, '{}')
    OR c.company_sizes  IS DISTINCT FROM csize_final.arr
    OR c.maturity       IS DISTINCT FROM mat.val);

-- name: CompanyJobCountBySlug :one
-- The denormalized open-job count for a slug (pgx.ErrNoRows if the company is
-- absent). cmd/import-yc uses it to guard against homonym collisions: it skips
-- enriching an existing company whose job_count dwarfs a matched YC entry's team.
SELECT job_count FROM companies WHERE slug = $1;
