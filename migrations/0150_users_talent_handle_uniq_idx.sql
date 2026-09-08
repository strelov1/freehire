-- migrate: no-transaction
--
-- Uniqueness for users.talent_handle (0149), and the index that serves the public
-- card route's `WHERE talent_handle = $1`. Same shape 0086/0118 established: a bare
-- unique index built CONCURRENTLY, not a named UNIQUE table constraint — a constraint
-- cannot be added concurrently, and adding it plainly would scan and lock `users`,
-- which is read and written on nearly every authenticated request.
--
-- The index is PARTIAL. Only members hold a handle; every other row is NULL, and a
-- full index over ~700k mostly-NULL rows is pages of nothing. `WHERE talent_handle IS
-- NOT NULL` also makes the intent legible: uniqueness is a property of minted handles,
-- and Postgres would not have compared the NULLs anyway.
--
-- Deliberately NO `IF NOT EXISTS`. A failed CONCURRENTLY build leaves an INVALID index
-- under this name, and a no-transaction file that errors is not recorded as applied, so
-- the next migrate run retries this file. With IF NOT EXISTS that retry would see the
-- name taken, skip silently, and record the migration as done — leaving prod with an
-- index that enforces nothing while the ledger claims it was built. That is exactly the
-- failure 0117 and 0118 existed to repair, and the whole reason this comment is here.
--
-- On an existing prod volume, run it DETACHED from the SSH session (systemd-run or
-- nohup): an attached build is how the 0086 carcass was created in the first place.
-- squawk-ignore prefer-robust-stmts -- IF NOT EXISTS is the bug here, not the fix: it makes the retry skip an INVALID index of the same name and record the migration as applied. See the paragraph above and migrations 0117/0118.
CREATE UNIQUE INDEX CONCURRENTLY users_talent_handle_key
    ON public.users (talent_handle)
    WHERE talent_handle IS NOT NULL;
