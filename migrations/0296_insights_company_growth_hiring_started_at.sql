-- insights_company_growth.hiring_started_at: when a company that had not been hiring started
-- again, for the "just started hiring" leaderboard (GET /api/v1/insights/companies?sort=new).
--
-- NULL for every company that is not a fresh starter; cmd/rollup-company sets it to the
-- earliest open posting of the new streak (see ComputeInsightsCompanyGrowth for the rules).
-- Nullable rather than a boolean so the leaderboard can order by recency without a join.
--
-- Expansive: apply before deploying the binary that writes and reads it. The table is one row
-- per company and rebuilt nightly, so the ALTER is metadata-only and instant.
ALTER TABLE public.insights_company_growth
    ADD COLUMN IF NOT EXISTS hiring_started_at timestamptz;
