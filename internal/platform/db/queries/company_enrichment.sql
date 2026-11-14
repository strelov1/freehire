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
--
-- Setting a non-empty industries here immediately zeroes industries_derived rather
-- than waiting for the next RefreshCompanyFacets: that column answers only where
-- industries is empty, and leaving a stale non-empty value in place would let a
-- company just curated by cmd/import-company-industries keep matching through
-- domains it no longer speaks for — the #2082 bug, reopened for the gap between
-- this write and the next periodic recompute (hours). The reverse case (industries
-- becomes empty) is left for that recompute like every other job-derived facet: it
-- only widens reach, never produces a false match, so it can stay eventually
-- consistent.
UPDATE companies
SET industries = sqlc.arg(industries),
    industries_derived = CASE
        WHEN cardinality(sqlc.arg(industries)::text[]) > 0 THEN '{}'::text[]
        ELSE industries_derived
    END,
    updated_at = now()
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
-- tagline fills only a blank, company_info merges key-wise, and industries union —
-- guarded the same way the four below are, once an active employer account exists: an
-- employer who deliberately REMOVES a tag via their profile edit must not see this
-- importer silently re-add it from the YC entry on its next scheduled run.
--
-- subindustry/year_founded/employee_count/hq_country are a FOURTH kind of not-owned:
-- unlike the three above, this importer WAS their only writer, right up until a verified
-- employer (internal/ingest/employer, migration 0174) became a second, more authoritative
-- one — the company speaking about itself outranks an imported directory entry. Since
-- there was never a second writer before, these four were never protected at all, so the
-- guard is a plain "leave them alone when an active company_accounts row exists for this
-- slug" rather than a merge: an employer's own edit is meant to win outright (see
-- add-employer-company-accounts' company-info spec delta), and this importer's job is
-- simply to not be the one that undoes it on its next scheduled run.
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
    -- overwriting — UNLESS an active employer account exists, in which case the stored
    -- value (the employer's own, curated list) wins outright, same as the four columns
    -- below. Sorted because the stored order is compared for equality by the
    -- normalization worker's no-op guard.
    industries      = CASE WHEN EXISTS (
                          SELECT 1 FROM company_accounts ca
                          WHERE ca.company_slug = companies.slug AND ca.status = 'active'
                      ) THEN companies.industries ELSE ARRAY(
        SELECT DISTINCT x
        FROM unnest(companies.industries || EXCLUDED.industries) AS x
        WHERE x <> ''
        ORDER BY x
    ) END,
    subindustry     = CASE WHEN EXISTS (
                          SELECT 1 FROM company_accounts ca
                          WHERE ca.company_slug = companies.slug AND ca.status = 'active'
                      ) THEN companies.subindustry ELSE EXCLUDED.subindustry END,
    year_founded    = CASE WHEN EXISTS (
                          SELECT 1 FROM company_accounts ca
                          WHERE ca.company_slug = companies.slug AND ca.status = 'active'
                      ) THEN companies.year_founded ELSE EXCLUDED.year_founded END,
    employee_count  = CASE WHEN EXISTS (
                          SELECT 1 FROM company_accounts ca
                          WHERE ca.company_slug = companies.slug AND ca.status = 'active'
                      ) THEN companies.employee_count ELSE EXCLUDED.employee_count END,
    hq_country      = CASE WHEN EXISTS (
                          SELECT 1 FROM company_accounts ca
                          WHERE ca.company_slug = companies.slug AND ca.status = 'active'
                      ) THEN companies.hq_country ELSE EXCLUDED.hq_country END,
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

-- name: FillCompanyDescriptionFromIngest :exec
-- Applies a company-level description an ATS adapter yielded alongside its board
-- crawl (see sources.CompanyDescriber — Greenhouse's board-metadata endpoint today).
-- Same fill-gap shape as UpsertYCCompany's non-owned columns: tagline fills only a
-- blank, company_info merges key-wise (existing keys win), touching nothing else —
-- this source has no industries/year_founded/etc. to assert. A slug with no existing
-- row is inserted with is_reference = false, since it is arriving with a real
-- crawled job, not as a reference-only row the way an unmatched YC entry is.
-- Also stamps company_description_checked_at: a write here IS an answered fetch.
INSERT INTO companies (
    slug, name, tagline, company_info, is_reference, company_info_at,
    company_description_checked_at
) VALUES (
    sqlc.arg(slug), sqlc.arg(name), sqlc.arg(tagline), sqlc.arg(company_info), false, now(),
    now()
)
ON CONFLICT (slug) DO UPDATE SET
    tagline         = COALESCE(NULLIF(companies.tagline, ''), EXCLUDED.tagline),
    company_info    = EXCLUDED.company_info || companies.company_info,
    company_info_at = now(),
    company_description_checked_at = now(),
    updated_at      = now();

-- name: MarkCompanyDescriptionChecked :exec
-- Records an answered company-description fetch that found the field BLANK, so the next
-- crawl does not ask again until the refresh window passes (see
-- pipeline.CompanyDescriptionRefreshWindow). Touches nothing else — company_info_at means
-- "info was written", and nothing was. Inserts the row like FillCompanyDescriptionFromIngest
-- does when the slug is new, because a stamp with nowhere to live would be lost and the
-- board asked again on every run.
INSERT INTO companies (slug, name, is_reference, company_description_checked_at)
VALUES (sqlc.arg(slug), sqlc.arg(name), false, now())
ON CONFLICT (slug) DO UPDATE SET
    company_description_checked_at = now();

-- name: CompanyDescriptionDue :one
-- Whether an ingest crawl should fetch this company's own description: it holds none yet
-- (company_info.summary absent or blank — the fill keeps a stored one, so re-asking could
-- not change it) AND no answered fetch has been recorded since checked_since. A slug with
-- no row at all is due.
SELECT NOT EXISTS (
    SELECT 1 FROM companies
    WHERE slug = sqlc.arg(slug)
      AND (
          COALESCE(company_info->>'summary', '') <> ''
          OR company_description_checked_at >= sqlc.arg(checked_since)::timestamptz
      )
) AS due;

-- name: ListCompaniesMissingWikipediaInfo :many
-- Candidates for the Wikipedia company-info backfill: no tagline yet, and never
-- resolved by this backfill before (company_info_wikipedia_checked_at IS NULL —
-- set on every resolution, match or reject, so a company is looked up at most
-- once). Keyset-paginated by slug so one run can be bounded and a later run
-- resumes past what it already paged through.
SELECT slug, name FROM companies
WHERE (tagline IS NULL OR tagline = '')
  AND company_info_wikipedia_checked_at IS NULL
  AND slug > sqlc.arg(after_slug)
ORDER BY slug
LIMIT sqlc.arg(row_limit);

-- name: FillCompanyInfoFromWikipedia :exec
-- Applies a confident Wikipedia match: fills tagline only if blank, merges the
-- company_info keys (existing keys win on collision, matching UpsertYCCompany's
-- gap-fill rule), and marks the company checked so it is never looked up again.
UPDATE companies
SET tagline          = COALESCE(NULLIF(tagline, ''), sqlc.arg(tagline)),
    company_info     = sqlc.arg(company_info) || company_info,
    company_info_at  = now(),
    company_info_wikipedia_checked_at = now(),
    updated_at       = now()
WHERE slug = sqlc.arg(slug);

-- name: MarkCompanyWikipediaChecked :exec
-- Records that the backfill looked this company up and found no confident match,
-- so it is never looked up again. Touches nothing else: an unmatched company's
-- tagline/company_info stay exactly as another source may have left them.
UPDATE companies
SET company_info_wikipedia_checked_at = now()
WHERE slug = sqlc.arg(slug);

-- name: ListUncategorizedOpenJobTitles :many
-- Keyset page (by id) of the open, canonical, public, bodied postings RefreshCompanyFacets'
-- category predicate would drop: no column category, no enrichment answer, not confirmed
-- technical. cmd/recount-companies runs search.TitleKeepsUncategorized over their titles and
-- hands the ids it keeps back as kept_ids, so the count follows search on the one branch
-- this SQL cannot derive (#4503). Paged so a run never holds the whole set at once.
SELECT j.id, j.title
FROM jobs j
WHERE j.id > sqlc.arg(after_id)
  AND j.closed_at IS NULL AND j.duplicate_of IS NULL AND NOT j.is_private
  AND j.company_slug <> '' AND j.description <> ''
  AND j.category = '' AND COALESCE(j.enrichment->>'category', '') IN ('', 'other')
  AND j.is_tech IS NOT TRUE
ORDER BY j.id
LIMIT sqlc.arg(batch_size);
