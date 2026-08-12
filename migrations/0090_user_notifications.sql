-- user_notifications is the in-app notification center's ledger: one row per
-- delivered subscription digest, reminder, or nudge, independent of which
-- channel(s) carried it (see the add-notification-center change). kind is
-- one of subscription_digest / reminder / nudge_follow_up /
-- nudge_interview_prep / nudge_job_closed. public_slug is nullable — a
-- subscription digest that matched more than one job carries no single job
-- to link to, matching the same "no deep link" rule the push channel already
-- uses for a multi-job digest. No internal job id is stored, same as
-- DigestJob's own wire shape: title/body already carry the job's
-- title/company as text, and nothing here needs a join back to jobs.

CREATE TABLE public.user_notifications (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    kind text NOT NULL,
    title text NOT NULL,
    body text NOT NULL,
    public_slug text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    read_at timestamp with time zone
);

ALTER TABLE public.user_notifications ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.user_notifications_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

ALTER TABLE ONLY public.user_notifications
    ADD CONSTRAINT user_notifications_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.user_notifications
    ADD CONSTRAINT user_notifications_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

-- Backs the list query (newest first, per user) and satisfies
-- TestEveryUserForeignKeyIsIndexed.
CREATE INDEX user_notifications_user_id_created_at_idx
    ON public.user_notifications (user_id, created_at DESC);

-- Backs the unread-count query without scanning read rows.
CREATE INDEX user_notifications_user_id_unread_idx
    ON public.user_notifications (user_id)
    WHERE read_at IS NULL;
