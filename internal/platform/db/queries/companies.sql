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
       feedback_count, feedback_rating_avg, ai_interview_reports
FROM companies
WHERE job_count > 0
  AND (sqlc.arg('search')::text = '' OR name ILIKE '%' || sqlc.arg('search') || '%' OR slug ILIKE '%' || sqlc.arg('search') || '%')
  AND (coalesce(cardinality(sqlc.arg('collections')::text[]), 0) = 0 OR collections && sqlc.arg('collections')::text[])
  AND (coalesce(cardinality(sqlc.arg('regions')::text[]), 0) = 0 OR regions && sqlc.arg('regions')::text[])
  AND (coalesce(cardinality(sqlc.arg('countries')::text[]), 0) = 0 OR countries && sqlc.arg('countries')::text[])
  AND (coalesce(cardinality(sqlc.arg('domains')::text[]), 0) = 0 OR domains && sqlc.arg('domains')::text[])
  -- industries answers from EITHER source: `industries` is what an importer wrote,
  -- `industries_derived` is RefreshCompanyFacets' materialized translation of the
  -- company's own domains — already empty for a company with a curated industry, or
  -- with more than two domains (a domains union that wide describes hiring range, not
  -- business; see the change design). Precedence and the domain-count threshold are
  -- baked in at materialization time, so this predicate is a plain OR, same shape as
  -- every other array facet.
  --
  -- It must exist on THIS path too: when only industries is set the request never
  -- reaches Meili, and a facet the fallback does not know is silently ignored.
  AND (coalesce(cardinality(sqlc.arg('industries')::text[]), 0) = 0
       OR industries && sqlc.arg('industries')::text[]
       OR industries_derived && sqlc.arg('industries')::text[])
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
-- internal/api/handler/companies.go) — rating is not (yet) a Meili-sortable
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
  -- industries answers from EITHER source: `industries` is what an importer wrote,
  -- `industries_derived` is RefreshCompanyFacets' materialized translation of the
  -- company's own domains — already empty for a company with a curated industry, or
  -- with more than two domains (a domains union that wide describes hiring range, not
  -- business; see the change design). Precedence and the domain-count threshold are
  -- baked in at materialization time, so this predicate is a plain OR, same shape as
  -- every other array facet.
  --
  -- It must exist on THIS path too: when only industries is set the request never
  -- reaches Meili, and a facet the fallback does not know is silently ignored.
  AND (coalesce(cardinality(sqlc.arg('industries')::text[]), 0) = 0
       OR industries && sqlc.arg('industries')::text[]
       OR industries_derived && sqlc.arg('industries')::text[])
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
--
-- website rides along for the same reason: a portfolio that publishes its members'
-- sites lets a same-named company with a DIFFERENT site be told apart from the member.
--
-- job_count rides along for a source that publishes its members' headcount: a
-- two-person startup cannot be the same-named company holding hundreds of postings.
SELECT slug, collections, countries, hq_country,
       COALESCE(company_info ->> 'website', '')::text AS website,
       job_count
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

-- name: ListSlugLikeCompaniesForBackfill :many
-- Companies whose ingested name is still a squished slug (lowercase, no
-- whitespace or uppercase) and that have at least one open job, with a
-- representative open job's source and URL so the backfill worker can locate the
-- ATS board. Only boards with live jobs matter, so dead ones never appear. The Go
-- side re-validates slug-likeness authoritatively before touching anything.
--
-- AND NOT is_private excludes the jd-tailor-intake private-job path: the representative
-- row here supplies the URL the worker reads a real display name from, and the name it
-- reads is written to the PUBLIC companies row (RenameSlugCompany below). A JD one user
-- pasted in is not a board, and it must not be the source of an employer's public name.
SELECT DISTINCT ON (company_slug)
       company_slug AS slug,
       company      AS name,
       source,
       url
FROM jobs
WHERE closed_at IS NULL
  AND duplicate_of IS NULL
  AND NOT is_private
  AND company_slug <> ''
  AND company ~ '^[a-z0-9._-]+$'
ORDER BY company_slug, created_at DESC;

-- name: RenameSlugCompany :execrows
-- Apply a resolved display name to every job under a slug-like company and
-- re-key its company_slug (computed by the caller via normalize.Slug); the caller then
-- reconciles the touched slugs through SyncCompaniesForSlugs + DeleteOrphanCompaniesForSlugs.
-- The name guard keeps a re-run from overwriting a name that is no longer a slug.
UPDATE jobs
SET company = @name,
    company_slug = @new_slug,
    -- Kept in step with company_slug by hand, like every other write path that sets
    -- it — see migrations/0109 for why the folded value is a stored column at all,
    -- and internal/platform/db/folded_slug_rule_test.go for the test that catches a new write
    -- path forgetting this line.
    company_slug_folded = replace(@new_slug, '-', ''),
    updated_at = now()
WHERE company_slug = @old_slug
  AND company ~ '^[a-z0-9._-]+$';

-- name: RenamePlaceholderBoardJobs :many
-- Apply a resolved display name to every stored job of ONE board whose company is still that
-- board's placeholder (see ListPlaceholderCompanyBoards). board_pattern is
-- externalid.BoardPattern(board), the board's external_id namespace, so only this board's rows
-- move. Needed beside the board write because a crawl only REWRITES a posting that changed — an
-- unchanged one is refreshed for liveness and keeps the placeholder, and a closed one is never
-- seen again. Guarded on the old name, so a re-run writes nothing. The moved rows' indexed
-- company and slug change, so an open, canonical, public one is queued for search in the same
-- statement, as every other writer of indexed job content does.
--
-- Returns the company_slug each moved job carried BEFORE the write, one row per job: the rows
-- are matched by board and name, not by slug, and an alias the merge worker recorded may have
-- re-keyed them away from the slug the placeholder name folds to — the caller sweeps exactly
-- these for orphaned companies rows.
WITH moved AS (
    SELECT j.id, j.company_slug
    FROM jobs j
    WHERE j.source = sqlc.arg(source)
      AND j.external_id LIKE sqlc.arg(board_pattern)
      AND j.company = sqlc.arg(old_name)
    FOR UPDATE
), updated AS (
    UPDATE jobs
    SET company = sqlc.arg(name),
        company_slug = sqlc.arg(new_slug),
        -- Kept in step with company_slug by hand; see RenameSlugCompany.
        company_slug_folded = replace(sqlc.arg(new_slug), '-', ''),
        updated_at = now()
    FROM moved
    WHERE jobs.id = moved.id
    RETURNING jobs.id, moved.company_slug AS old_slug, COALESCE(jobs.posted_at, jobs.created_at) AS eff_posted_at,
        jobs.closed_at IS NULL AND jobs.duplicate_of IS NULL AND NOT jobs.is_private AS indexed
), queued AS (
    INSERT INTO search_outbox (job_id, job_posted_at)
    SELECT id, eff_posted_at FROM updated WHERE indexed
    -- Re-stamp a live entry as RequeueSearchOutbox does: a drain completing a claim it took
    -- before the rename deletes by the old id and must not take the rename's entry with it.
    ON CONFLICT (job_id) DO UPDATE
        SET id = DEFAULT, created_at = now(), attempts = 0, last_error = '', failed_at = NULL
)
SELECT old_slug FROM updated;

-- name: ListPlaceholderBoardDescriptions :many
-- A few of one board's stored posting bodies, open ones first, for a placeholder whose ATS
-- states the employer nowhere but in the posting text (adp; see cmd/backfill-company-names).
-- board_pattern is externalid.BoardPattern(board). A board is small — the index behind
-- (source, external_id LIKE) reaches its rows directly — so the sort is over one board only.
SELECT description
FROM jobs
WHERE source = sqlc.arg(source)
  AND external_id LIKE sqlc.arg(board_pattern)
  AND description <> ''
ORDER BY (closed_at IS NULL) DESC, id DESC
LIMIT sqlc.arg(max_rows);

-- name: CompanyNamesBySlugs :many
-- Display names for a bounded set of slugs, in one read. GET /jobs/facets names
-- the top companies of its company_slug distribution (at most 50) with it. A slug
-- with no row yet (the companies table trails ingest) is simply absent.
SELECT slug, name
FROM companies
WHERE slug = ANY(sqlc.arg(slugs)::text[]);

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

-- name: CompanyJobCountBySlug :one
-- The denormalized open-job count for a slug (pgx.ErrNoRows if the company is
-- absent). cmd/import-yc uses it to guard against homonym collisions: it skips
-- enriching an existing company whose job_count dwarfs a matched YC entry's team.
SELECT job_count FROM companies WHERE slug = $1;

-- name: SetCompanyAccountProfile :exec
-- A verified employer's authoritative edit to their own company's curated profile
-- (internal/ingest/employer, PATCH /employer/company) — see company-info's "verified
-- employer... authoritative" spec delta. Unlike every other writer of these columns
-- (cmd/import-yc, the Wikipedia backfill, ingest's adapter-supplied description), this one
-- REPLACES rather than merges or fills a gap: the employer is the company speaking about
-- itself, strictly more authoritative than an imported or inferred source.
--
-- A NULL scalar argument means "the request did not touch this field" (COALESCE keeps the
-- stored value) — the caller passes only what was actually supplied, nil-means-unchanged
-- like every other patch in this codebase. company_info_patch is a plain JSONB merge with
-- the new value winning on key collision (the opposite direction from cmd/import-yc's
-- fill-gap merge): pass '{}' when the request touches neither description nor website, so
-- the merge is a no-op and every other key already stored (funding, parent_company, …) is
-- left alone.
--
-- industries is deliberately NOT here — it goes through the existing SetCompanyIndustries,
-- which replaces the whole array (not a per-employer union), and is called separately by
-- the service only when the request actually supplies industries.
UPDATE companies
SET tagline         = COALESCE(sqlc.narg(tagline), tagline),
    company_info    = company_info || sqlc.arg(company_info_patch)::jsonb,
    year_founded    = COALESCE(sqlc.narg(year_founded), year_founded),
    employee_count  = COALESCE(sqlc.narg(employee_count), employee_count),
    hq_country      = COALESCE(sqlc.narg(hq_country), hq_country),
    subindustry     = COALESCE(sqlc.narg(subindustry), subindustry),
    company_info_at = now(),
    updated_at      = now()
WHERE slug = sqlc.arg(slug);

-- name: SeedCompanyAccountWebsite :exec
-- internal/ingest/employer's fill-only-if-blank website seed: a moderator approving a
-- pending employer-account claim whose domain the automatic check could not itself verify
-- (see employer-account's spec). A new slug is inserted as an is_reference row (mirroring
-- cmd/import-yc's own pattern for a company with no jobs yet, migration 0174's comment);
-- an existing row's company_info gets the "website" key ONLY when absent or blank — the
-- WHERE clause on the UPDATE branch is the guard, not merely a defensive no-op, since this
-- must never overwrite a value another source (or the employer's own later curated edit)
-- already asserted.
INSERT INTO companies (slug, name, company_info, is_reference, company_info_at)
VALUES (
    sqlc.arg(slug), sqlc.arg(name),
    jsonb_build_object('website', sqlc.arg(website)::text),
    true, now()
)
ON CONFLICT (slug) DO UPDATE SET
    company_info    = companies.company_info || jsonb_build_object('website', sqlc.arg(website)::text),
    company_info_at = now(),
    updated_at      = now()
WHERE NOT (companies.company_info ? 'website') OR companies.company_info ->> 'website' = '';

-- name: InsertMissingCompaniesForSlugs :execrows
-- A companies row for each of the given slugs that jobs carry and companies lacks: the
-- scoped half of the reconcile cmd/backfill-derive runs after it re-keys jobs onto new slugs.
-- The caller passes the slugs it moved, a bounded chunk at a time, so each statement is its
-- own short transaction. SyncCompaniesFromJobs did this for the whole table in ONE
-- transaction that ran about an hour, and a row it inserted stayed locked until it committed
-- — so every ingest UpsertJob for that company, which upserts the same row, queued behind it
-- (2026-10-28/29, up to 140 waiters).
--
-- The name is the most recent public one, as ListCompaniesForMerge reads it; a JD one user
-- pasted in (is_private) must not name a public company.
INSERT INTO companies (slug, name)
SELECT DISTINCT ON (j.company_slug) j.company_slug, j.company
FROM jobs j
WHERE j.company_slug = ANY(sqlc.arg(slugs)::text[])
  AND NOT j.is_private
  AND NOT EXISTS (SELECT 1 FROM companies c WHERE c.slug = j.company_slug)
ORDER BY j.company_slug, j.created_at DESC, j.id DESC
ON CONFLICT (slug) DO NOTHING;

-- name: DeleteOrphanCompaniesForSlugs :execrows
-- DeleteOrphanCompanies narrowed to the given slugs: the other half of cmd/backfill-derive's
-- scoped reconcile, for the slugs its re-key moved jobs AWAY from. Reference rows stay.
DELETE FROM companies c
WHERE c.slug = ANY(sqlc.arg(slugs)::text[])
  AND NOT c.is_reference
  AND NOT EXISTS (SELECT 1 FROM jobs j WHERE j.company_slug = c.slug);

-- name: SyncCompaniesForSlugs :execrows
-- SyncCompaniesFromJobs narrowed to the given slugs: gives each a companies row and the
-- name its jobs now carry. cmd/backfill-company-names passes the slugs it renamed jobs onto,
-- a bounded chunk at a time, so each statement is its own short transaction. It used to run
-- the whole-table sync after every run — an hour in one transaction, holding a lock on
-- every row it wrote against ingest's UpsertJob and blocking any migration on jobs — even
-- on a run that renamed nothing (2026-10-29: every model call refused with 429, then the
-- sync held jobs for an hour and kept migration 0212 out).
--
-- Unlike InsertMissingCompaniesForSlugs this also rewrites the NAME of an existing row: a
-- rename that keeps its slug ("afcb" -> "AFCB") changes nothing else. The filter sits
-- outside the ON CONFLICT for the reason SyncCompaniesFromJobs gives. The name is the most
-- recent public one; a JD one user pasted in (is_private) must not name a public company.
INSERT INTO companies (slug, name)
SELECT j.company_slug, j.company
FROM (
    SELECT DISTINCT ON (company_slug) company_slug, company
    FROM jobs
    WHERE company_slug = ANY(sqlc.arg(slugs)::text[])
      AND NOT is_private
    ORDER BY company_slug, created_at DESC, id DESC
) j
WHERE NOT EXISTS (
    SELECT 1 FROM companies c WHERE c.slug = j.company_slug AND c.name = j.company
)
ON CONFLICT (slug) DO UPDATE SET
    name       = EXCLUDED.name,
    updated_at = now();

-- name: ListHiringCompaniesByFoldedSlug :many
-- The companies carrying searchable jobs whose slug folds to one of the given keys, or IS one
-- of the given slugs — the candidates the "Hiring again" filter (internal/job/formeremployer)
-- chooses among. Both halves are needed: a name resolves either through the alias registry,
-- whose canonical slug need not fold to the name's own key (a curated merge), or through the
-- fold itself, where EVERY slug sharing the key must come back so a key two companies share
-- can be refused rather than half-answered.
--
-- The keys are joined through unnest rather than compared with `= ANY`, so each key is one
-- probe of companies_folded_slug_idx (migration 0222) — see that file for why the `= ANY`
-- form over an expression is the shape that went wrong on jobs.
SELECT c.slug, c.countries
FROM unnest(@folded_keys::text[]) AS k(key)
JOIN companies c ON replace(c.slug, '-', '') = k.key
WHERE c.job_count > 0
UNION
SELECT c.slug, c.countries
FROM companies c
WHERE c.slug = ANY(@slugs::text[])
  AND c.job_count > 0;

-- name: ListCompaniesBySiteHost :many
-- Every company whose site (company_info website, else homepage) reduces to one of the
-- given hosts, job_count 0 included. Rides companies_site_host_idx.
SELECT company_site_host(company_info)::text AS domain,
       slug, name, job_count,
       COALESCE(NULLIF(regexp_replace(company_info ->> 'website', '^[ \t\n\v\f\r]+|[ \t\n\v\f\r]+$', '', 'g'), ''), NULLIF(regexp_replace(company_info ->> 'homepage', '^[ \t\n\v\f\r]+|[ \t\n\v\f\r]+$', '', 'g'), ''))::text AS website
FROM companies
WHERE company_site_host(company_info) = ANY(sqlc.arg(hosts)::text[])
ORDER BY domain, job_count DESC, slug;
