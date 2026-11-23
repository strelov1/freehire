-- Publication-based counts for the last 12 completed UTC weeks. The worker
-- replaces the small snapshot atomically, including explicit zero-count weeks.
CREATE TABLE public.insights_role_demand_weekly (
    category text NOT NULL,
    week date NOT NULL,
    new_jobs bigint NOT NULL CHECK (new_jobs >= 0),
    PRIMARY KEY (category, week)
);
