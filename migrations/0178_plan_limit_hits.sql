-- plan_limit_hits records every time internal/ai/plan.Store.Consume refuses a user
-- for a real plan ceiling — never for the fair-use guard (FairUse), which only an
-- unlimited (paying) tier can reach, and never while a feature runs unenforced
-- (Shadowed), which lets the request through. It is the one place that hit is
-- observable server-side; see cmd/limit-nudge-mail, whose candidate query reads it
-- (scoped to free-tier accounts only, even though a paid tier with a real per-feature
-- ceiling — e.g. auto-apply on Pro — can land a row here too) to find accounts worth
-- telling that Pro removes the ceiling.
--
-- Keyed by (user_id, feature, day) with ON CONFLICT DO NOTHING rather than
-- append-only: the nudge only needs to know THAT a wall was hit on a given day, not
-- how many times, and an append-only table would grow one row per refused request
-- for an account that kept retrying the same action.
CREATE TABLE public.plan_limit_hits (
    user_id bigint NOT NULL,
    feature text NOT NULL,
    day date NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT plan_limit_hits_pkey PRIMARY KEY (user_id, feature, day),
    CONSTRAINT plan_limit_hits_user_id_fkey FOREIGN KEY (user_id)
        REFERENCES public.users(id) ON DELETE CASCADE
);

COMMENT ON TABLE public.plan_limit_hits IS
    'One row per (user, feature, day) a real plan-ceiling refusal was recorded (any '
    'tier). Feeds cmd/limit-nudge-mail''s candidate query, which filters to free tier.';

-- users.limit_nudge_sent_at is the one-time send ledger for that nudge, mirroring
-- pro_welcome_sent_at (migration 0160): NULL until sent, never cleared — hitting the
-- wall again after the nudge already landed is not a reason to mail it twice.
ALTER TABLE users ADD COLUMN limit_nudge_sent_at timestamptz;

COMMENT ON COLUMN users.limit_nudge_sent_at IS
    'When the one-time "you ran into today''s limit" nudge was sent. NULL until sent; '
    'never cleared once set.';
