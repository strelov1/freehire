CREATE TABLE public.discord_links (
    user_id    bigint NOT NULL,
    discord_id bigint NOT NULL,
    linked_at  timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE ONLY public.discord_links
    ADD CONSTRAINT discord_links_pkey PRIMARY KEY (user_id);

ALTER TABLE ONLY public.discord_links
    ADD CONSTRAINT discord_links_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE public.link_contributions
    DROP CONSTRAINT link_contributions_surface_check;

ALTER TABLE public.link_contributions
    ADD CONSTRAINT link_contributions_surface_check
    CHECK ((surface = ANY (ARRAY['web'::text, 'telegram'::text, 'extension'::text, 'cli'::text, 'discord'::text, 'unknown'::text])));
