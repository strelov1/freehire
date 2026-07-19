-- Saved-job reminders: an opt-in, one-shot nudge to come back to a saved job
-- before it goes stale. See the add-saved-job-reminders change.
--
-- reminder_settings is the per-user default rule: whether reminders are on, the
-- default delay applied to new saves, and which channels to deliver over. An
-- absent row means the feature was never configured, so it reads as disabled —
-- no backfill needed to keep reminders off for existing users.
--
-- job_reminders is the schedule AND the delivery ledger, one row per scheduled
-- reminder. It mirrors subscription_matches' crash-safe delivery bookkeeping
-- (claimed_at lease + reaper, attempts/failed_at dead-letter) but is driven by a
-- fire_at deadline rather than a filter match, and carries a status enum because
-- a reminder has two terminal states (delivered, cancelled) not one. channels is
-- snapshotted from the rule at schedule time so a later rule edit never rewrites
-- a pending reminder. At most one pending reminder exists per (user, job).
--
-- Applied to a fresh volume by initdb after 0033; on an existing prod volume these
-- statements must be run manually BEFORE deploying code that reads the tables.

CREATE TABLE public.reminder_settings (
    user_id bigint NOT NULL,
    enabled boolean DEFAULT false NOT NULL,
    default_delay_days integer DEFAULT 3 NOT NULL,
    channels text[] DEFAULT '{}'::text[] NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE ONLY public.reminder_settings
    ADD CONSTRAINT reminder_settings_pkey PRIMARY KEY (user_id);

ALTER TABLE ONLY public.reminder_settings
    ADD CONSTRAINT reminder_settings_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

CREATE TABLE public.job_reminders (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    job_id bigint NOT NULL,
    fire_at timestamp with time zone NOT NULL,
    channels text[] NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    claimed_at timestamp with time zone,
    attempts integer DEFAULT 0 NOT NULL,
    failed_at timestamp with time zone,
    last_error text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    delivered_at timestamp with time zone,
    CONSTRAINT job_reminders_status_check CHECK (status IN ('pending', 'delivered', 'cancelled'))
);

ALTER TABLE public.job_reminders ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.job_reminders_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

ALTER TABLE ONLY public.job_reminders
    ADD CONSTRAINT job_reminders_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.job_reminders
    ADD CONSTRAINT job_reminders_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.job_reminders
    ADD CONSTRAINT job_reminders_job_id_fkey FOREIGN KEY (job_id) REFERENCES public.jobs(id) ON DELETE CASCADE;

-- At most one pending reminder per (user, job): re-saving a job upserts against
-- this index, and delivered/cancelled history rows are exempt so they never block
-- a fresh schedule.
CREATE UNIQUE INDEX job_reminders_pending_uniq
    ON public.job_reminders (user_id, job_id) WHERE status = 'pending';

-- The worker's due-scan: pending, not-yet-dead reminders ordered by deadline.
CREATE INDEX job_reminders_due_idx
    ON public.job_reminders (fire_at) WHERE status = 'pending' AND failed_at IS NULL;

-- Job-close cancellation cancels every pending reminder for a closing job by job_id.
CREATE INDEX job_reminders_job_pending_idx
    ON public.job_reminders (job_id) WHERE status = 'pending';
