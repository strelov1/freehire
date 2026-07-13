-- Deterministic company maturity facet (see the company-classification-facets change).
-- maturity is a single-valued classification on the company's lifecycle stage —
-- 'government' | 'startup' | 'scaleup' | 'enterprise' — derived deterministically by
-- RefreshCompanyFacets / cmd/recount-companies from signals already stored
-- (organization_type, yc_status, employee_count, year_founded, and whether the
-- company's jobs come from an exclusively-government source like usajobs/neogov).
--
-- Unlike the array facets (yc_*, regions, company_types), maturity is a SCALAR text
-- column and is NULLABLE on purpose: NULL means "unknown" (an honest abstain when no
-- signal fits), and it filters by membership (maturity = ANY(...)) rather than array
-- overlap. It replaces the ambiguous single company_type only on the maturity axis;
-- company_type itself is left untouched (deprecation is a later change).
--
-- Applied to a fresh volume by initdb after 0016; on an existing prod volume this
-- statement must be run manually BEFORE deploying code that reads the column, then
-- cmd/recount-companies rematerializes every company.

ALTER TABLE public.companies
    ADD COLUMN maturity text;
