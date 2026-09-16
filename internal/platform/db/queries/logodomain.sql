-- ListCompanyWebsites returns the companies whose curated record holds a website, which
-- is the only population cmd/publish-logo-domains can publish a domain for. ~17,900 rows
-- of ~480,000 companies as of 2026-09-16.
--
-- name is selected beside the website because the company surfaces (/companies, the
-- company header, the company picker) ask the logo proxy with companies.name rather than
-- with any posting's spelling of it, and that value is not guaranteed to appear in jobs.
--
-- name: ListCompanyWebsites :many
SELECT slug, name, (company_info ->> 'website')::text AS website
FROM companies
WHERE company_info ? 'website'
  AND company_info ->> 'website' <> ''
  AND job_count > 0;

-- ListOpenJobCompanySpellings returns every distinct way an open posting spells its
-- company's name. 412,648 rows as of 2026-09-16.
--
-- This is a deliberate SEQUENTIAL SCAN of jobs, and the narrower-looking alternative is
-- four times slower. Measured on production 2026-09-16:
--
--   this query                                        53s  (seq scan)
--   the same joined to the 17,859 companies above    200s  (index nested loop)
--
-- Restricting to the companies we can publish drives 17,859 index searches, each fetching
-- ~102 heap rows at random: 1.59M blocks of random I/O against the seq scan's sequential
-- read of the same heap. Fewer rows, more work. The caller filters in Go instead.
--
-- name: ListOpenJobCompanySpellings :many
SELECT DISTINCT company_slug, company
FROM jobs
WHERE closed_at IS NULL
  AND company_slug <> ''
  AND company <> '';
