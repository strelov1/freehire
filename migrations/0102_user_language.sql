-- migrate: no-transaction
--
-- The account's preferred interface language: first step toward interface i18n
-- (see users.timezone in 0094 for the same "store the preference before the
-- feature exists" pattern). Unlike timezone, the option set is small and curated
-- rather than an open namespace, so it gets a CHECK constraint instead of
-- application-only validation, and a NOT NULL DEFAULT — every account has a
-- language from the moment it exists, no "unset" state to special-case.
--
-- users is one of the hottest tables in the app, so this follows 0085's split
-- rather than a plain ADD CONSTRAINT: ADD COLUMN with a non-volatile default is
-- metadata-only (PG11+), ADD ... NOT VALID skips the validation scan, and
-- VALIDATE CONSTRAINT takes only SHARE UPDATE EXCLUSIVE, blocking neither
-- readers nor writers. Validation cannot fail: every existing row's default is
-- 'en', which the CHECK permits.
ALTER TABLE public.users
    ADD COLUMN language text NOT NULL DEFAULT 'en';

ALTER TABLE public.users
    ADD CONSTRAINT users_language_check
    CHECK (language = ANY (ARRAY['en'::text, 'ru'::text, 'es'::text, 'pt'::text, 'de'::text, 'fr'::text])) NOT VALID;

ALTER TABLE public.users
    VALIDATE CONSTRAINT users_language_check;
