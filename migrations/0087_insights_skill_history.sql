-- Weekly skill-demand history, backing the personalized GET /me/market-pulse
-- read. insights_skill_stats (0022) is fully cleared and reinserted on every
-- cmd/rollup-stats run, so it can only ever answer "now vs. one fixed window
-- back" — there is nowhere in it to read a multi-point trend from. This table
-- is the append-only companion: one row per canonical skill per ISO week,
-- carrying that skill's open_count as of the run that first observed the
-- week (subsequent runs within the same week are no-ops via the writer's
-- ON CONFLICT DO NOTHING, not this schema).
--
-- Only the global (category='', country='') skill bucket is snapshotted —
-- see design.md's Non-Goals. Retention (~26 weeks) is enforced by the writer
-- pruning old rows each run, not by anything in this schema.
--
-- Applied to a fresh volume by initdb after 0086; on an existing prod volume
-- run this manually before deploying code that reads it (per the migrations
-- gotcha).

CREATE TABLE public.insights_skill_history (
    skill      text    NOT NULL,
    week_start date    NOT NULL,
    open_count integer NOT NULL,
    PRIMARY KEY (skill, week_start)
);

-- Serves "all retained weeks for a set of skills, newest first" — the read
-- GET /me/market-pulse issues for the caller's own profile skills.
CREATE INDEX insights_skill_history_skill_week_idx
    ON public.insights_skill_history (skill, week_start DESC);
