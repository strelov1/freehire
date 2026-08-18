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
-- (partial index) instead of scanning the full 2.3 GB heap. job_count counts the
-- postings the JOB SEARCH INDEX holds (see RefreshCompanyFacets), so the number here
-- is the number the company's own page shows — the two used to disagree, listing
-- Stripe at 570 against 444 on its page.
SELECT slug, name, job_count, tagline, industries, hq_country, collections,
       feedback_count, feedback_rating_avg
FROM companies
WHERE job_count > 0
  AND (sqlc.arg('search')::text = '' OR name ILIKE '%' || sqlc.arg('search') || '%' OR slug ILIKE '%' || sqlc.arg('search') || '%')
  AND (coalesce(cardinality(sqlc.arg('collections')::text[]), 0) = 0 OR collections && sqlc.arg('collections')::text[])
  AND (coalesce(cardinality(sqlc.arg('regions')::text[]), 0) = 0 OR regions && sqlc.arg('regions')::text[])
  AND (coalesce(cardinality(sqlc.arg('countries')::text[]), 0) = 0 OR countries && sqlc.arg('countries')::text[])
  AND (coalesce(cardinality(sqlc.arg('domains')::text[]), 0) = 0 OR domains && sqlc.arg('domains')::text[])
  -- industries answers from EITHER source, which is why two arrays arrive for one
  -- facet: `industries` is what an importer wrote, `industry_domains` is the caller's
  -- industries translated into the coarse job-derived vocabulary by
  -- internal/industrytag, matched against the domains the company's own postings
  -- imply. The curated column covers 27% of the catalogue, so the second arm is most
  -- of the facet's reach, not a fallback. An industry the mapping does not cover
  -- contributes nothing to the second array, and `domains && '{}'` is false, so the
  -- curated arm answers alone without a special case.
  --
  -- It must exist on THIS path too: when only industries is set the request never
  -- reaches Meili, and a facet the fallback does not know is silently ignored.
  AND (coalesce(cardinality(sqlc.arg('industries')::text[]), 0) = 0
       OR industries && sqlc.arg('industries')::text[]
       -- The derived arm answers only where the curated one is SILENT. The two are
       -- not equal evidence: `domains` is a union over every open job the company
       -- holds, so for a company with hundreds of postings it drifts from what the
       -- company is toward the range of work it advertises — Uber accumulates
       -- gamedev, edtech and govtech that way, and briefly answered
       -- ?industries=gaming in production because of it. Consulting that union for a
       -- company an importer has already classified adds no reach (its own values
       -- already match it) and asserts industries it is not in.
       OR (cardinality(industries) = 0
           AND domains && sqlc.arg('industry_domains')::text[]))
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
-- `sort = 'rating'` orders by the materialized feedback_rating_avg (unrated
-- companies sort last), falling through to the default job_count DESC, name
-- for the tiebreak. Any other value (including '', the default) leaves the
-- CASE NULL for every row, so this ORDER BY is byte-for-byte the old one.
-- sort=rating forces every request onto this Postgres path even with a search
-- or facet present that would otherwise route to Meili (see ListCompanies in
-- internal/handler/companies.go) — rating is not (yet) a Meili-sortable
-- attribute, so routing there would silently drop the requested order.
ORDER BY
  CASE WHEN sqlc.arg('sort')::text = 'rating' THEN feedback_rating_avg END DESC NULLS LAST,
  job_count DESC, name
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
  -- industries answers from EITHER source, which is why two arrays arrive for one
  -- facet: `industries` is what an importer wrote, `industry_domains` is the caller's
  -- industries translated into the coarse job-derived vocabulary by
  -- internal/industrytag, matched against the domains the company's own postings
  -- imply. The curated column covers 27% of the catalogue, so the second arm is most
  -- of the facet's reach, not a fallback. An industry the mapping does not cover
  -- contributes nothing to the second array, and `domains && '{}'` is false, so the
  -- curated arm answers alone without a special case.
  --
  -- It must exist on THIS path too: when only industries is set the request never
  -- reaches Meili, and a facet the fallback does not know is silently ignored.
  AND (coalesce(cardinality(sqlc.arg('industries')::text[]), 0) = 0
       OR industries && sqlc.arg('industries')::text[]
       -- The derived arm answers only where the curated one is SILENT. The two are
       -- not equal evidence: `domains` is a union over every open job the company
       -- holds, so for a company with hundreds of postings it drifts from what the
       -- company is toward the range of work it advertises — Uber accumulates
       -- gamedev, edtech and govtech that way, and briefly answered
       -- ?industries=gaming in production because of it. Consulting that union for a
       -- company an importer has already classified adds no reach (its own values
       -- already match it) and asserts industries it is not in.
       OR (cardinality(industries) = 0
           AND domains && sqlc.arg('industry_domains')::text[]))
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

-- name: ListCompaniesForReindex :many
-- Keyset page of hiring companies (job_count > 0) for the companies search reindex,
-- cursored by the slug primary key (first chunk keyed by the empty string, which
-- sorts before every slug). SELECT * so the row stays db.Company as columns grow and
-- search.FromCompany can map every facet. The job_count > 0 scope keeps the index to
-- companies that are actually hiring, matching the /companies list's hiring scope, and
-- rides companies_hiring_job_count_idx instead of scanning the full heap.
SELECT *
FROM companies
WHERE slug > sqlc.arg(after_slug) AND job_count > 0
ORDER BY slug
LIMIT sqlc.arg(batch_size);

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
--
-- countries and hq_country ride along for the credential gates: a register entry is
-- only granted to a company demonstrably present in that register's country, and a
-- single-token name additionally needs its headquarters there. Both are already
-- maintained (countries by RefreshCompanyFacets, hq_country by cmd/import-yc), so
-- this widens the read rather than adding a source of truth.
SELECT slug, collections, countries, hq_country
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
SET company = @name,
    company_slug = @new_slug,
    -- Kept in step with company_slug by hand, like every other write path that sets
    -- it — see migrations/0109 for why the folded value is a stored column at all,
    -- and internal/db/folded_slug_rule_test.go for the test that catches a new write
    -- path forgetting this line.
    company_slug_folded = replace(@new_slug, '-', ''),
    updated_at = now()
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

-- name: ListCompanySlugs :many
-- Every company slug, unfiltered. cmd/import-yc loads this once into an
-- in-memory set to resolve each yc-oss directory entry's current-name and
-- former-name slug candidates, instead of one CompanyExists round trip per
-- candidate per entry (the dataset runs several thousand entries deep).
SELECT slug FROM companies;

-- name: ListCompanyIndustriesPage :many
-- Keyset page over every company, ordered by slug so a run resumes from the last
-- slug it saw. Deliberately unfiltered: the normalization pass only cares about
-- rows that already hold industries, but the merge pass must also reach companies
-- with none, and one query serving both keeps the two walks identical.
SELECT slug, industries
FROM companies
WHERE slug > sqlc.arg(after_slug)
ORDER BY slug
LIMIT sqlc.arg(page_limit);

-- name: SetCompanyIndustries :execrows
-- Replace one company's industries. The IS DISTINCT FROM guard keeps updated_at
-- honest — a row already holding the wanted value is not rewritten — and makes the
-- affected-row count real churn, so a second run reports zero.
UPDATE companies
SET industries = sqlc.arg(industries), updated_at = now()
WHERE slug = sqlc.arg(slug) AND industries IS DISTINCT FROM sqlc.arg(industries);

-- name: UpsertYCCompany :exec
-- Apply one yc-oss directory entry, matched by slug. A new slug is inserted as a
-- reference row (is_reference = true) with no jobs; an existing slug (job-backed or a
-- prior reference) has the YC-owned columns refreshed — name, job_count, collections,
-- is_reference, and the job-derived facet arrays (regions/remote_regions/countries/
-- domains/company_types/company_sizes) are left untouched. Idempotent: re-running
-- the same entry rewrites the same values.
--
-- Three columns are NOT YC-owned, because this is no longer their only writer, and
-- replacing them would erase another source's work on the importer's next run:
-- tagline fills only a blank, company_info merges key-wise, and industries union.
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
    -- Union, sorted and de-duplicated, so two sources accumulate instead of
    -- overwriting. Sorted because the stored order is compared for equality by the
    -- normalization worker's no-op guard.
    industries      = ARRAY(
        SELECT DISTINCT x
        FROM unnest(companies.industries || EXCLUDED.industries) AS x
        WHERE x <> ''
        ORDER BY x
    ),
    subindustry     = EXCLUDED.subindustry,
    year_founded    = EXCLUDED.year_founded,
    employee_count  = EXCLUDED.employee_count,
    hq_country      = EXCLUDED.hq_country,
    -- NULLIF folds '' into NULL so an empty string counts as absent, not as a value
    -- worth protecting.
    tagline         = COALESCE(NULLIF(companies.tagline, ''), EXCLUDED.tagline),
    -- Operand order is load-bearing: a || b keeps b on key collision, so the YC keys
    -- fill gaps while anything already stored wins. Reversed, this is the bug above.
    company_info    = EXCLUDED.company_info || companies.company_info,
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
-- oj is referenced by all eight aggregates, so it is pinned MATERIALIZED: without the
-- keyword the planner is free to inline it and re-scan the open-jobs set per aggregate.
--
-- `oj` is scoped to the postings the JOB SEARCH INDEX will hold, which is what every
-- consumer of these columns actually means. job_count gates the companies index and so
-- the sitemap, the facet arrays are what the /companies filters offer, and a company
-- page's list is served by search — so a row search drops must not put a company in
-- the sitemap, under a filter, or behind a number its own page contradicts. Counting
-- the wider table scope did all three: 294,021 company URLs in the sitemap of which
-- most rendered "0 open jobs", `remote_regions` offering regions whose jobs the click
-- through could not find, and Stripe listed at 570 on /companies against 444 on its
-- own page. The three predicates below are the ones cmd/reindex's splitJobs applies
-- on top of closed/duplicate; keep the two in step.
WITH oj AS MATERIALIZED (
    -- duplicate_of IS NULL counts one canonical job per role cluster, so the company
    -- job_count matches the collapsed /jobs and company lists (reposts share facets, so
    -- the DISTINCT region/country aggregates are unaffected — only the count changes).
    SELECT company_slug, regions, countries, enrichment, work_mode, source
    FROM jobs
    WHERE closed_at IS NULL AND duplicate_of IS NULL AND company_slug <> ''
      -- Visible only to its creator (the jd-tailor-intake path).
      AND NOT is_private
      -- search.DescriptionMissing strips the body to plain text first, which SQL
      -- cannot reproduce, so a body that is only empty markup still counts here. That
      -- residual is small and self-limiting: such a company reaches the sitemap, and
      -- its page answers noindex off the same search that excluded the row.
      AND description <> ''
      -- search.CategoryUnresolved: the column answers, or the enrichment does — and
      -- "other" is the classifier declining, not an answer. This is the predicate that
      -- excluded almost everything in the sample, the dictionaries being deliberately
      -- dict-only: a Samara kindergarten's vacancy resolves to no tech category, so
      -- nobody can find it and its employer is not a page worth handing a crawler.
      AND (category <> '' OR COALESCE(enrichment->>'category', '') NOT IN ('', 'other'))
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
