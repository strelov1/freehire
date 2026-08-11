-- user_push_tokens is the mobile-app device push registry: one row per
-- installed-app instance (a device), not per user. A push token identifies
-- the install, so it is upserted by the token itself (ON CONFLICT (token))
-- rather than unique on (user_id, token) — if a different account signs in
-- on the same device, the row reassigns to the new owner instead of leaving
-- the previous owner with a live token to a device they're no longer signed
-- into. See the push-notification-infra change.

CREATE TABLE public.user_push_tokens (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    token text NOT NULL,
    platform text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    last_seen_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT user_push_tokens_platform_check CHECK (platform IN ('ios', 'android'))
);

ALTER TABLE public.user_push_tokens ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.user_push_tokens_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

ALTER TABLE ONLY public.user_push_tokens
    ADD CONSTRAINT user_push_tokens_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.user_push_tokens
    ADD CONSTRAINT user_push_tokens_token_key UNIQUE (token);

ALTER TABLE ONLY public.user_push_tokens
    ADD CONSTRAINT user_push_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

-- Required alongside the FK: Postgres indexes only the referenced side, so
-- without this, deleting a user would scan this whole table (see
-- TestEveryUserForeignKeyIsIndexed). Also backs ListPushTokensForUser.
CREATE INDEX user_push_tokens_user_id_idx
    ON public.user_push_tokens (user_id);
