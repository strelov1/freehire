-- The account's preferred interface language: first step toward interface i18n
-- (see users.timezone in 0094 for the same "store the preference before the
-- feature exists" pattern). Unlike timezone, the option set is small and curated
-- rather than an open namespace, so it gets a CHECK constraint instead of
-- application-only validation, and a NOT NULL DEFAULT — every account has a
-- language from the moment it exists, no "unset" state to special-case.
ALTER TABLE public.users
    ADD COLUMN language text NOT NULL DEFAULT 'en',
    ADD CONSTRAINT users_language_check
        CHECK (language = ANY (ARRAY['en'::text, 'ru'::text, 'es'::text, 'pt'::text, 'de'::text, 'fr'::text]));
