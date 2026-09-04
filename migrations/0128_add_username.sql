-- migrate: no-transaction
-- squawk-ignore-file prefer-robust-stmts -- the ADD CONSTRAINT .. NOT VALID / VALIDATE CONSTRAINT pair below cannot be made re-runnable via IF NOT EXISTS (Postgres has no such clause for ADD CONSTRAINT), and this is a deliberately non-transactional file, so squawk's default "wrap it and a partial failure is harmless" reasoning does not apply either. The per-statement `-- squawk-ignore prefer-robust-stmts` form does not suppress this rule in squawk-cli 2.63.0 (verified empirically), hence the file-level form.
--
-- Account-level username (see the add-username-claim change): a single, unique,
-- user-visible name the hosted mailbox adopts instead of allocating its own handle,
-- and that a future talent-network change can expose as a public profile URL.
--
-- users is one of the hottest tables in the app (read/written on nearly every
-- authenticated request) — the same reasoning 0085/0071 already document applies
-- here: ADD COLUMN with no default is metadata-only on any PG version regardless of
-- volatility, so that part is instant either way. The CHECK constraint is the part
-- that needs care — a plain `ADD CONSTRAINT ... CHECK` scans every existing row
-- while holding the lock ADD CONSTRAINT takes, and wrapped in this runner's default
-- transaction that lock would be held for the whole scan. So: ADD ... NOT VALID
-- (skips the scan, metadata-only) then a separate VALIDATE CONSTRAINT (takes only
-- SHARE UPDATE EXCLUSIVE, blocking neither readers nor writers). Validation cannot
-- fail here: every existing row's username is NULL, and a CHECK always treats NULL
-- as satisfied.
--
-- Uniqueness is enforced by a CONCURRENTLY-built index in a companion migration
-- (0129_users_username_uniq_idx.sql) for the identical reason 0086 splits out
-- talent_network_public_id's uniqueness — CONCURRENTLY cannot run inside a
-- transaction block, and this file already carries other statements. Apply 0129
-- BEFORE running the cmd/backfill-username-from-mailbox worker, not after — see
-- 0129's header for why the other order silently breaks the backfill's own
-- collision resolution.
--
-- Applied to a fresh volume by initdb after 0127; on an existing prod volume run
-- this manually (SET ROLE hire) BEFORE deploying code that reads it.

ALTER TABLE public.users
    ADD COLUMN IF NOT EXISTS username text,
    ADD COLUMN IF NOT EXISTS username_updated_at timestamptz;

ALTER TABLE public.users
    ADD CONSTRAINT users_username_check
    CHECK (char_length(username) BETWEEN 3 AND 30 AND username ~ '^[a-z0-9]+(-[a-z0-9]+)*$') NOT VALID;

ALTER TABLE public.users
    VALIDATE CONSTRAINT users_username_check;
