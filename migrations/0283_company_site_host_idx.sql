-- migrate: no-transaction
-- Data API: find companies by web domain. company_site_host reduces a company's site
-- (company_info website, else homepage) to a bare host: lowercase, no scheme, userinfo,
-- port, path, trailing dot or leading www. The Go normalizer (normalizeSiteHost in
-- internal/api/handler/companies_by_domain.go) must stay in agreement with it.
-- Built concurrently: companies is ~1M rows.
CREATE OR REPLACE FUNCTION public.company_site_host(info jsonb) RETURNS text
    LANGUAGE sql IMMUTABLE PARALLEL SAFE
    AS $$
        SELECT NULLIF(
            regexp_replace(
            regexp_replace(
            regexp_replace(
            regexp_replace(
            regexp_replace(
            regexp_replace(
                lower(COALESCE(NULLIF(regexp_replace(info ->> 'website', '^[ \t\n\v\f\r]+|[ \t\n\v\f\r]+$', '', 'g'), ''), NULLIF(regexp_replace(info ->> 'homepage', '^[ \t\n\v\f\r]+|[ \t\n\v\f\r]+$', '', 'g'), ''), '')),
                '^[a-z][a-z0-9+.-]*://', ''),
                '[/?#].*$', ''),
                '^.*@', ''),
                ':[0-9]*$', ''),
                '[.]+$', ''),
                '^www[.]', ''),
            '')
    $$;

CREATE INDEX CONCURRENTLY IF NOT EXISTS companies_site_host_idx
    ON public.companies (public.company_site_host(company_info));
