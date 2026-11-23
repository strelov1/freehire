-- name: ComputeCompanyJobFacets :many
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
-- (eventual consistency).
--
-- It is two statements so the jobs scan can run on the analytics replica: this one reads
-- jobs/job_activity only and returns one row per company with open jobs (counts, the
-- jobs-derived facet arrays and the gov-source marker); ApplyCompanyFacets below takes
-- those rows as JSON on the primary and does everything that reads `companies`. Before
-- the split the single UPDATE wrote ~1.5 GB of temp files per run on the primary. The facet aggregates are each their own non-correlated
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
-- own page. The three predicates below (private, body, unseen) are the ones
-- cmd/reindex's splitJobs applies on top of closed/duplicate; keep the two in step.
--
-- industries_derived (see derived_eligible/derived_ind below) is a SECOND-ORDER
-- derivation — computed from this pass's own `dom` and the company's curated
-- `industries`, not from `jobs` directly — so it is not one of the eight oj-scoped
-- aggregates above. `mapping_domains`/`mapping_industries` are
-- internal/dict/industrytag.DomainIndustryPairs(), passed as parameters so this query
-- has exactly one copy of the domain→industry table to read, not a second one typed
-- into SQL.
--
-- `unseen_sources`/`unseen_seconds` are search.Withheld's unseen rule as parallel arrays
-- (internal/ingest/sources.UnseenWindows, resolved by the caller): a posting whose source is
-- listed and has not shown it for longer than that many seconds is not in search, so it is
-- not counted here either. A source not listed is never excluded on this ground — the same
-- reading search gives it.
WITH unseen_window AS (
    -- Zipped by position for the same analyzer reason as domain_industry_map below.
    SELECT s.source, w.seconds
    FROM unnest(sqlc.arg('unseen_sources')::text[]) WITH ORDINALITY AS s(source, ord)
    JOIN unnest(sqlc.arg('unseen_seconds')::bigint[]) WITH ORDINALITY AS w(seconds, ord)
      ON w.ord = s.ord
),
oj AS MATERIALIZED (
    -- duplicate_of IS NULL counts one canonical job per role cluster, so the company
    -- job_count matches the collapsed /jobs and company lists (reposts share facets, so
    -- the DISTINCT region/country aggregates are unaffected — only the count changes).
    SELECT j.company_slug, j.regions, j.countries, j.enrichment, j.work_mode, j.source
    FROM jobs j
    LEFT JOIN unseen_window uw ON uw.source = j.source
    LEFT JOIN job_activity a ON a.job_id = j.id
    WHERE j.closed_at IS NULL AND j.duplicate_of IS NULL AND j.company_slug <> ''
      -- search.UnseenWindows.Unseen: the source has stopped showing the posting. last_seen_at is
      -- job_activity's; a job without an activity row reads as unseen, only for a windowed source.
      AND (uw.source IS NULL OR a.last_seen_at >= now() - make_interval(secs => uw.seconds))
      -- Visible only to its creator (the jd-tailor-intake path).
      AND NOT j.is_private
      -- search.DescriptionMissing strips the body to plain text first, which SQL
      -- cannot reproduce, so a body that is only empty markup still counts here. That
      -- residual is small and self-limiting: such a company reaches the sitemap, and
      -- its page answers noindex off the same search that excluded the row.
      AND j.description <> ''
      -- No category predicate: search indexes an uncategorized posting under "other" (#4412).
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
)
-- Every company in oj has a counts row, so counts drives; the arrays are never NULL.
SELECT counts.company_slug,
       counts.cnt::bigint                  AS job_count,
       gov.is_gov::boolean                 AS is_gov,
       COALESCE(reg.arr, '{}')::text[]        AS regions,
       COALESCE(remote_reg.arr, '{}')::text[] AS remote_regions,
       COALESCE(cty.arr, '{}')::text[]        AS countries,
       COALESCE(dom.arr, '{}')::text[]        AS domains,
       COALESCE(ctype.arr, '{}')::text[]      AS company_types,
       COALESCE(csize.arr, '{}')::text[]      AS company_sizes
FROM counts
JOIN gov             ON gov.company_slug        = counts.company_slug
LEFT JOIN reg        ON reg.company_slug        = counts.company_slug
LEFT JOIN remote_reg ON remote_reg.company_slug = counts.company_slug
LEFT JOIN cty        ON cty.company_slug        = counts.company_slug
LEFT JOIN dom        ON dom.company_slug        = counts.company_slug
LEFT JOIN ctype      ON ctype.company_slug      = counts.company_slug
LEFT JOIN csize      ON csize.company_slug      = counts.company_slug;

-- name: ApplyCompanyFacets :execrows
-- The write half of the recount (see ComputeCompanyJobFacets above for the contract).
-- `facets` is ComputeCompanyJobFacets' rows as a JSON array of objects keyed by its
-- column names; a company absent from it has no open jobs and is zeroed/emptied. The
-- rest reads only `companies`, so it stays cheap on the primary.
-- Each array is re-sorted by value, the order ComputeCompanyJobFacets aggregated it in, so
-- the IS DISTINCT FROM guard compares like with like and an unchanged row is not rewritten.
WITH f AS (
    SELECT e->>'company_slug' AS company_slug,
           (e->>'job_count')::bigint AS cnt,
           (e->>'is_gov')::boolean AS is_gov,
           ARRAY(SELECT v FROM jsonb_array_elements_text(e->'regions') AS v ORDER BY v) AS regions,
           ARRAY(SELECT v FROM jsonb_array_elements_text(e->'remote_regions') AS v ORDER BY v) AS remote_regions,
           ARRAY(SELECT v FROM jsonb_array_elements_text(e->'countries') AS v ORDER BY v) AS countries,
           ARRAY(SELECT v FROM jsonb_array_elements_text(e->'domains') AS v ORDER BY v) AS domains,
           ARRAY(SELECT v FROM jsonb_array_elements_text(e->'company_types') AS v ORDER BY v) AS company_types,
           ARRAY(SELECT v FROM jsonb_array_elements_text(e->'company_sizes') AS v ORDER BY v) AS company_sizes
    FROM jsonb_array_elements(sqlc.arg('facets')::jsonb) AS e
),
counts AS (SELECT company_slug, cnt FROM f),
reg AS (SELECT company_slug, regions AS arr FROM f WHERE cardinality(regions) > 0),
remote_reg AS (SELECT company_slug, remote_regions AS arr FROM f WHERE cardinality(remote_regions) > 0),
cty AS (SELECT company_slug, countries AS arr FROM f WHERE cardinality(countries) > 0),
dom AS (SELECT company_slug, domains AS arr FROM f WHERE cardinality(domains) > 0),
ctype AS (SELECT company_slug, company_types AS arr FROM f WHERE cardinality(company_types) > 0),
csize AS (SELECT company_slug, company_sizes AS arr FROM f WHERE cardinality(company_sizes) > 0),
gov AS (SELECT company_slug, is_gov FROM f),
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
),
-- derived_eligible is the set of companies the industries_derived arm may speak for
-- at all: no curated industry of their own (co.industries empty — precedence, unless
-- from #2082) AND at most two distinct domains (the #2088 threshold — above that, the
-- domains union describes hiring range rather than business, see the change design).
-- A company outside this set gets industries_derived = '{}' by simply not appearing
-- in derived_ind below.
derived_eligible AS (
    SELECT co.slug AS company_slug, COALESCE(dom.arr, '{}') AS domains
    FROM companies co
    LEFT JOIN dom ON dom.company_slug = co.slug
    WHERE cardinality(co.industries) = 0
      AND cardinality(COALESCE(dom.arr, '{}')) <= 2
),
-- Zips the two parallel parameter arrays back into (domain, industry) rows by
-- position (WITH ORDINALITY), since this analyzer's catalog does not resolve the
-- two-array form of unnest(text[], text[]) the way a live Postgres does. The pairs
-- themselves are internal/dict/industrytag.DomainIndustryPairs(), passed in rather
-- than duplicated as a second copy of the table in SQL.
domain_industry_map AS (
    SELECT d.domain, i.industry
    FROM unnest(sqlc.arg('mapping_domains')::text[]) WITH ORDINALITY AS d(domain, ord)
    JOIN unnest(sqlc.arg('mapping_industries')::text[]) WITH ORDINALITY AS i(industry, ord)
      ON i.ord = d.ord
),
-- Translates each eligible company's domains to industries via the mapping above. A
-- company whose domains all fail to join (none of them map to an industry, e.g.
-- {other} or a retired vocabulary value) simply produces no rows here and reads as
-- '{}' below, with no special case.
derived_ind AS (
    SELECT de.company_slug,
           array_agg(DISTINCT map.industry ORDER BY map.industry) AS arr
    FROM derived_eligible de
    CROSS JOIN LATERAL unnest(de.domains) AS d(domain)
    JOIN domain_industry_map map ON map.domain = d.domain
    GROUP BY de.company_slug
)
UPDATE companies c
SET job_count          = COALESCE(counts.cnt, 0),
    regions            = COALESCE(reg.arr, '{}'),
    remote_regions     = COALESCE(remote_reg.arr, '{}'),
    countries          = COALESCE(cty.arr, '{}'),
    domains            = COALESCE(dom.arr, '{}'),
    company_types      = COALESCE(ctype.arr, '{}'),
    company_sizes      = csize_final.arr,
    maturity           = mat.val,
    industries_derived = COALESCE(derived_ind.arr, '{}')
FROM companies c2
LEFT JOIN counts      ON counts.company_slug     = c2.slug
LEFT JOIN reg         ON reg.company_slug        = c2.slug
LEFT JOIN remote_reg  ON remote_reg.company_slug = c2.slug
LEFT JOIN cty         ON cty.company_slug        = c2.slug
LEFT JOIN dom         ON dom.company_slug        = c2.slug
LEFT JOIN ctype       ON ctype.company_slug      = c2.slug
LEFT JOIN csize_final ON csize_final.company_slug = c2.slug
LEFT JOIN mat         ON mat.company_slug        = c2.slug
LEFT JOIN derived_ind ON derived_ind.company_slug = c2.slug
WHERE c.slug = c2.slug
  AND (c.job_count          IS DISTINCT FROM COALESCE(counts.cnt, 0)
    OR c.regions            IS DISTINCT FROM COALESCE(reg.arr, '{}')
    OR c.remote_regions     IS DISTINCT FROM COALESCE(remote_reg.arr, '{}')
    OR c.countries          IS DISTINCT FROM COALESCE(cty.arr, '{}')
    OR c.domains            IS DISTINCT FROM COALESCE(dom.arr, '{}')
    OR c.company_types      IS DISTINCT FROM COALESCE(ctype.arr, '{}')
    OR c.company_sizes      IS DISTINCT FROM csize_final.arr
    OR c.maturity           IS DISTINCT FROM mat.val
    OR c.industries_derived IS DISTINCT FROM COALESCE(derived_ind.arr, '{}'));
