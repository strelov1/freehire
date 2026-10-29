-- Agent views: postings served to agents over MCP, OJCP and /api/v1/agent/jobs/search,
-- counted by cmd/rollup-views from internal/application/agentview's log. Kept apart from
-- `uniques` (people and API reads, from nginx) and `page_uniques` (people only) because
-- jobs.view_count now sums both sources, and once summed nothing else could tell an
-- agent's view from a person's.
-- squawk-ignore prefer-bigint-over-int -- a per-(day, job) count, the same integer type as the sibling uniques/page_uniques columns it sits beside; nowhere near int32's range
ALTER TABLE job_daily_views ADD COLUMN agent_uniques integer NOT NULL DEFAULT 0;
