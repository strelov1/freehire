-- Bulk publication of results computed on the analytics read connection.

-- name: CopyJobDailyStats :copyfrom
INSERT INTO job_daily_stats (day, added, removed, computed_at)
VALUES (sqlc.arg('day'), sqlc.arg('added'), sqlc.arg('removed'), sqlc.arg('computed_at'));

-- name: CopyInsightsRoleStats :copyfrom
INSERT INTO insights_role_stats (category, seniority, country, open_count, open_count_prev)
VALUES (sqlc.arg('category'), sqlc.arg('seniority'), sqlc.arg('country'), sqlc.arg('open_count'), sqlc.arg('open_count_prev'));

-- name: CopyInsightsRoleCountryStats :copyfrom
INSERT INTO insights_role_country_stats (category, country, open_count, open_count_prev)
VALUES (sqlc.arg('category'), sqlc.arg('country'), sqlc.arg('open_count'), sqlc.arg('open_count_prev'));

-- name: CopyInsightsRolePulse :copyfrom
INSERT INTO insights_role_pulse (category, views_through, views, views_prev, exposure_days, exposure_days_prev, closed, closed_prev, median_close_days, median_close_days_prev, inflow)
VALUES (sqlc.arg('category'), sqlc.arg('views_through'), sqlc.arg('views'), sqlc.arg('views_prev'), sqlc.arg('exposure_days'), sqlc.arg('exposure_days_prev'), sqlc.arg('closed'), sqlc.arg('closed_prev'), sqlc.arg('median_close_days'), sqlc.arg('median_close_days_prev'), sqlc.arg('inflow'));

-- name: CopyInsightsRoleSkillStats :copyfrom
INSERT INTO insights_role_skill_stats (category, seniority, skill, open_count)
VALUES (sqlc.arg('category'), sqlc.arg('seniority'), sqlc.arg('skill'), sqlc.arg('open_count'));

-- name: CopyInsightsRoleSkillSample :copyfrom
INSERT INTO insights_role_skill_sample (category, seniority, sample_size)
VALUES (sqlc.arg('category'), sqlc.arg('seniority'), sqlc.arg('sample_size'));

-- name: CopyInsightsSkillStats :copyfrom
INSERT INTO insights_skill_stats (skill, category, country, open_count, open_count_prev)
VALUES (sqlc.arg('skill'), sqlc.arg('category'), sqlc.arg('country'), sqlc.arg('open_count'), sqlc.arg('open_count_prev'));

-- name: CopyInsightsSalaryStats :copyfrom
INSERT INTO insights_salary_stats (category, seniority, country, currency, period, sample_size, p25, p50, p75, quantiles)
VALUES (sqlc.arg('category'), sqlc.arg('seniority'), sqlc.arg('country'), sqlc.arg('currency'), sqlc.arg('period'), sqlc.arg('sample_size'), sqlc.arg('p25'), sqlc.arg('p50'), sqlc.arg('p75'), sqlc.arg('quantiles'));

-- name: CopyInsightsSalaryMonthly :copyfrom
INSERT INTO insights_salary_monthly (category, seniority, country, currency, period, month, sample_size, p25, p50, p75)
VALUES (sqlc.arg('category'), sqlc.arg('seniority'), sqlc.arg('country'), sqlc.arg('currency'), sqlc.arg('period'), sqlc.arg('month'), sqlc.arg('sample_size'), sqlc.arg('p25'), sqlc.arg('p50'), sqlc.arg('p75'));

-- name: CopyInsightsVelocityDaily :copyfrom
INSERT INTO insights_velocity_daily (day, facet_kind, facet_value, added, removed)
VALUES (sqlc.arg('day'), sqlc.arg('facet_kind'), sqlc.arg('facet_value'), sqlc.arg('added'), sqlc.arg('removed'));

-- name: CopyInsightsCompanyGrowth :copyfrom
INSERT INTO insights_company_growth (company_slug, open_count, open_count_prev)
VALUES (sqlc.arg('company_slug'), sqlc.arg('open_count'), sqlc.arg('open_count_prev'));

-- name: CopyInsightsCompanyResponse :copyfrom
INSERT INTO insights_company_response (company_slug, applications, answered, median_reply_days)
VALUES (sqlc.arg('company_slug'), sqlc.arg('applications'), sqlc.arg('answered'), sqlc.arg('median_reply_days'));

-- name: CopyInsightsCompanyLifetime :copyfrom
INSERT INTO insights_company_lifetime (company_slug, closed, median_days)
VALUES (sqlc.arg('company_slug'), sqlc.arg('closed'), sqlc.arg('median_days'));

-- name: CopyInsightsCompanyViews :copyfrom
INSERT INTO insights_company_views (company_slug, views, views_prev)
VALUES (sqlc.arg('company_slug'), sqlc.arg('views'), sqlc.arg('views_prev'));
