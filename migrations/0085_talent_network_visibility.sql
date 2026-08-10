-- Talent Network opt-in visibility (see the talent-network-profile-visibility
-- change). Two new, feature-scoped columns on users:
--
-- talent_network_visibility: a tri-state flag ('off' / 'public' / 'anonymous'),
-- same CHECK-constraint shape the rest of the schema already uses for scalar
-- enums with no independent lifecycle (application_nudges.kind,
-- api_keys.scope, ...). Defaults to 'off' — opting in is an explicit act.
--
-- talent_network_public_id: a random UUID for the public profile URL, minted
-- for every existing row via DEFAULT so no backfill is needed. This is NOT a
-- primary-key swap (unlike 0045's cvs.id) — users.id keeps its bigint
-- identity; this is purely an additive, feature-scoped opaque identifier, so
-- the public route never has to expose the sequential id.

ALTER TABLE public.users
    ADD COLUMN talent_network_visibility text DEFAULT 'off'::text NOT NULL,
    ADD COLUMN talent_network_public_id uuid DEFAULT gen_random_uuid() NOT NULL;

ALTER TABLE public.users
    ADD CONSTRAINT users_talent_network_visibility_check
    CHECK (talent_network_visibility IN ('off', 'public', 'anonymous'));

ALTER TABLE public.users
    ADD CONSTRAINT users_talent_network_public_id_key UNIQUE (talent_network_public_id);
