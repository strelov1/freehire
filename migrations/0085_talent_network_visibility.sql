-- migrate: no-transaction
--
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
-- the public route never has to expose the sequential id. Its uniqueness is
-- enforced by a CONCURRENTLY-built index in the companion migration
-- (0086_talent_network_public_id_uniq_idx.sql) rather than an inline UNIQUE
-- here — see that file for why it has to be its own no-transaction file.
--
-- WHY no-transaction, and why the CHECK is split. users is one of the
-- hottest tables in the app — read and written on nearly every authenticated
-- request — so it gets the same treatment 0071 (jobs.closed_reason) gives a
-- large, hot table: a plain `ADD CONSTRAINT ... CHECK` validates every
-- existing row while holding ACCESS EXCLUSIVE, and lock_timeout does not
-- help (internal/migrate bounds how long a migration WAITS for a lock, never
-- how long a statement HOLDS one). Wrapped in the runner's default
-- transaction, that lock would additionally be held across that whole scan,
-- and Postgres queues every reader/writer that arrives behind it — the shape
-- of the 2026-07-30 incident 0071 documents.
--
-- So: run the statements outside a transaction, and use the split 0049/0071
-- already name as the remedy. ADD COLUMN with a non-volatile default
-- ('off'::text) is metadata-only (PG11+), ADD ... NOT VALID skips the scan,
-- and VALIDATE CONSTRAINT takes only SHARE UPDATE EXCLUSIVE, which blocks
-- neither readers nor writers. Validation cannot fail: every existing row's
-- default is 'off', which the CHECK permits.
--
-- Applied to a fresh volume by initdb after 0084; on an existing prod volume
-- run this manually (SET ROLE hire) BEFORE deploying code that reads it.

ALTER TABLE public.users
    ADD COLUMN talent_network_visibility text DEFAULT 'off'::text NOT NULL,
    ADD COLUMN talent_network_public_id uuid DEFAULT gen_random_uuid() NOT NULL;

ALTER TABLE public.users
    ADD CONSTRAINT users_talent_network_visibility_check
    CHECK (talent_network_visibility IN ('off', 'public', 'anonymous')) NOT VALID;

ALTER TABLE public.users
    VALIDATE CONSTRAINT users_talent_network_visibility_check;
