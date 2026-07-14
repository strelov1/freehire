-- Clean YC subindustry classification (see the company-subindustry-facet change).
-- subindustry is the leaf of the YC subindustry path (e.g.
-- 'Industrials -> Manufacturing and Robotics' -> 'Manufacturing and Robotics'),
-- populated by cmd/import-yc from the yc-oss directory entry. It is stored SEPARATELY
-- from the flattened, tag-inclusive companies.industries display array, so the noisy
-- free tags never reach the filter: the industry FACET reads this clean column while
-- the display chips keep using industries.
--
-- Like maturity, subindustry is a SCALAR text column, NULLABLE on purpose: NULL means
-- "no clean subindustry" (a non-YC company, or a YC entry without one) and filters by
-- membership (subindustry = ANY(...)) rather than array overlap — never guessed.
--
-- Applied to a fresh volume by initdb after 0017; on an existing prod volume this
-- statement must be run manually BEFORE deploying code that reads the column, then
-- cmd/import-yc is re-run to populate every YC company.

ALTER TABLE public.companies
    ADD COLUMN subindustry text;
