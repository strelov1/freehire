-- migrate: no-transaction
--
-- GIN index for companies.industries_derived (0142), mirroring companies_industries_idx
-- and companies_domains_idx (0001_init.sql) — queried the same way: array-contains
-- equality against a requested `industries` value.
--
-- CONCURRENTLY, and split into its own no-transaction file, for the same reason 0078/
-- 0081/0093 give: companies is under continuous prod write traffic (RefreshCompanyFacets,
-- ingest) and a plain CREATE INDEX holds a SHARE lock blocking writes for the whole
-- build; Postgres forbids CONCURRENTLY inside a transaction block, and a migration
-- file's statements sent as one multi-statement query run in an implicit transaction
-- regardless of the no-transaction marker (that marker only stops internal/migrate's
-- OWN wrapping BEGIN/COMMIT) — so this has to be the only statement in its file.
--
-- Applied to a fresh volume by initdb after 0142; on an existing prod volume build it
-- by hand, detached from the SSH session (systemd-run or nohup) — a CONCURRENTLY
-- build dies with its ssh session and leaves an INVALID index behind, the same
-- warning 0078/0081/0093 already give.
CREATE INDEX CONCURRENTLY IF NOT EXISTS companies_industries_derived_idx
    ON public.companies USING gin (industries_derived);
