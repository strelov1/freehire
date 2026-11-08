-- paywall_hits records every time the paid-AI gate (internal/api/handler/paid_ai.go) refuses
-- a signed-in user who is on no paid plan: the free user who pressed a paid AI button and
-- got a 402. It is the warmest buying signal the product has, and until now it left only a
-- log line — plan_limit_hits never sees it, because plan.decide refuses the free tier before
-- any allowance is read, so there is no ceiling to hit. The two tables answer different
-- questions: plan_limit_hits is "a metered ceiling was reached", this is "the plan never
-- included the feature".
--
-- Written best-effort from the refusal path, off the request, by plan.Store.RecordPaywallHit;
-- read by an operator today (query below) and by the paywall letter later.
--
-- feature is the plan feature name for a metered route (tailor, match, assistant, ...) and
-- a fixed name for the three routes with no allowance of their own (ats_review, mail_recall,
-- autofill). job_id is set only when the action concerns one vacancy, and is resolved from a
-- bound identifier inside the insert: an unknown or foreign one leaves it NULL.
--
-- Keyed by (user, feature, job, day) with ON CONFLICT DO NOTHING rather than append-only: "who
-- hit which wall" needs THAT it happened and when it first did, not how many times, and an
-- append-only table would let one free account grow it without bound by retrying or by
-- iterating job slugs. A NULL job counts as one job (COALESCE to 0, which no jobs row has).
-- ON DELETE SET NULL on job_id could in principle fold two rows of one day into the same
-- key, but nothing hard-deletes jobs (see AGENTS.md, "Job deletion").
--
-- user_id cascades, so deleting an account erases what it looked at.
CREATE TABLE public.paywall_hits (
    id bigint GENERATED ALWAYS AS IDENTITY NOT NULL,
    user_id bigint NOT NULL,
    feature text NOT NULL,
    job_id bigint,
    day date NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT paywall_hits_pkey PRIMARY KEY (id),
    CONSTRAINT paywall_hits_user_id_fkey FOREIGN KEY (user_id)
        REFERENCES public.users(id) ON DELETE CASCADE,
    CONSTRAINT paywall_hits_job_id_fkey FOREIGN KEY (job_id)
        REFERENCES public.jobs(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX paywall_hits_user_feature_job_day_uniq
    ON public.paywall_hits (user_id, feature, COALESCE(job_id, 0), day);

-- The operator report reads a time window grouped by feature.
CREATE INDEX paywall_hits_feature_created_at_idx
    ON public.paywall_hits (feature, created_at);

-- The jobs FK is checked on every jobs delete and update of the key, and a cascade or SET NULL
-- needs to find the rows; a partial index keeps it small.
CREATE INDEX paywall_hits_job_id_idx
    ON public.paywall_hits (job_id) WHERE job_id IS NOT NULL;

COMMENT ON TABLE public.paywall_hits IS
    'One row per (user, feature, job, day) a free user was refused by the paid-AI gate '
    '(402). Operator report, last 7 days by feature: SELECT h.feature, h.user_id, '
    'j.public_slug, h.day, h.created_at FROM paywall_hits h LEFT JOIN jobs j ON j.id = '
    'h.job_id WHERE h.created_at >= now() - interval ''7 days'' ORDER BY h.feature, '
    'h.created_at DESC. Not plan_limit_hits: that one is a metered ceiling being reached.';
