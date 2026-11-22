-- migrate: no-transaction
-- The expression index 0109 replaced with the company_slug_folded column and its
-- jobs_open_company_slug_folded_col_idx. No scan on the primary since 2026-10-24 and
-- none on the replica; 418 MB written on every insert of an open posting.
DROP INDEX CONCURRENTLY IF EXISTS jobs_open_company_slug_folded_idx;
