-- application_nudges is the internal/nudge decision layer's MATCH ledger + delivery
-- bookkeeping, for the two lifecycle nudges: follow-up (an application has gone
-- silent past its stage's threshold) and interview-prep (a stage_set moved an
-- application into `interview`). Mirrors job_reminders' shape (0034): a status
-- enum with two terminal states (delivered, cancelled — a matched nudge whose
-- trigger no longer holds by delivery time is cancelled, not delivered) plus the
-- claimed_at lease / attempts / failed_at dead-letter bookkeeping shared with
-- subscription_matches and job_reminders.
--
-- episode_key is the idempotency key together with (user_id, job_id, kind): it is
-- the fact that must change before a re-notify is warranted (the application's
-- last_activity_at for follow_up, the triggering stage_set event's occurred_at for
-- interview_prep) rather than a snooze interval or a notified-count. See the
-- centralize-lifecycle-notifications change.

CREATE TABLE public.application_nudges (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    job_id bigint NOT NULL,
    kind text NOT NULL,
    episode_key timestamp with time zone NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    claimed_at timestamp with time zone,
    attempts integer DEFAULT 0 NOT NULL,
    failed_at timestamp with time zone,
    last_error text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    delivered_at timestamp with time zone,
    CONSTRAINT application_nudges_kind_check CHECK (kind IN ('follow_up', 'interview_prep')),
    CONSTRAINT application_nudges_status_check CHECK (status IN ('pending', 'delivered', 'cancelled', 'failed'))
);

ALTER TABLE public.application_nudges ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.application_nudges_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

ALTER TABLE ONLY public.application_nudges
    ADD CONSTRAINT application_nudges_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.application_nudges
    ADD CONSTRAINT application_nudges_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.application_nudges
    ADD CONSTRAINT application_nudges_job_id_fkey FOREIGN KEY (job_id) REFERENCES public.jobs(id) ON DELETE CASCADE;

-- The idempotency key: MATCH inserts ON CONFLICT DO NOTHING against this index, so
-- a re-scan of the same unchanged episode is a no-op.
CREATE UNIQUE INDEX application_nudges_episode_uniq
    ON public.application_nudges (user_id, job_id, kind, episode_key);

-- The worker's due-scan: pending, not-yet-dead nudges.
CREATE INDEX application_nudges_pending_idx
    ON public.application_nudges (id) WHERE status = 'pending' AND failed_at IS NULL;
