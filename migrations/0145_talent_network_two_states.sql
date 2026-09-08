-- migrate: no-transaction
--
-- The Talent Network's visibility becomes a two-state opt-in: 'off' or
-- 'anonymous'. The third state, 'public' (migration 0085), is retired — see
-- openspec/changes/talent-network-public-catalog. The product no longer asks a
-- candidate how much of themselves to disclose; the public projection is fixed
-- and anonymised, and joining is the only decision left to make.
--
-- Existing 'public' rows become 'anonymous', never 'off'. The rewrite may only
-- ever narrow what is disclosed about a person; it must not remove them from a
-- network they chose to be in.
--
-- WHY no-transaction, and why the constraint is added before the rewrite.
--
-- `users` is one of the hottest tables in the app — read and written on nearly
-- every authenticated request — so this file follows 0085's own reasoning: a
-- plain `ADD CONSTRAINT ... CHECK` validates every existing row while holding
-- ACCESS EXCLUSIVE, and internal/platform/migrate bounds how long a migration
-- WAITS for a lock, never how long a statement HOLDS one. Wrapped in the
-- runner's default transaction that lock would additionally be held across the
-- whole scan, with every reader and writer queued behind it.
--
-- The ORDER below closes a race the obvious order leaves open. A migration runs
-- BEFORE the code that knows about it is deployed, so while these statements
-- run, the currently-deployed server still believes 'public' is a legal value
-- and may write one. Rewriting first and constraining afterwards leaves a window
-- in which a fresh 'public' row lands between the two statements — and then
-- VALIDATE fails and the whole migration does.
--
-- So the NOT VALID constraint goes on FIRST. It performs no scan (that is what
-- NOT VALID means) and takes ACCESS EXCLUSIVE only for the instant it takes to
-- record the catalogue entry, but from that instant no 'public' row can be
-- written by anyone. The rewrite then runs against a set that cannot grow, and
-- VALIDATE — which takes only SHARE UPDATE EXCLUSIVE and blocks neither readers
-- nor writers — cannot fail.
--
-- The new constraint takes a NEW name rather than reusing 0085's, because both
-- exist at once between the first and last statements here. The old one is
-- dropped at the end, with IF EXISTS so that a re-run of a partially applied
-- file is clean.
--
-- The UPDATE is not chunked. It is bounded by its own predicate to the accounts
-- that chose the retired state — a set in the single digits, since the control
-- that sets it has never been reachable from the account navigation. This file
-- is not a precedent for an unchunked UPDATE over a large slice of `users`.
--
-- Applied to a fresh volume by initdb after 0144; on an existing prod volume run
-- this manually (SET ROLE hire) BEFORE deploying the code that reads it.

-- Clears the carcass a run that failed after the ADD would have left, so this
-- file is re-runnable despite having no transaction to roll back. Postgres has
-- no ADD CONSTRAINT IF NOT EXISTS, so making the ADD robust means dropping
-- first, not guarding in place.
ALTER TABLE public.users
    DROP CONSTRAINT IF EXISTS users_talent_network_visibility_two_states;

-- squawk-ignore prefer-robust-stmts -- rerunnable via the DROP IF EXISTS above; there is no ADD CONSTRAINT IF NOT EXISTS to use instead.
ALTER TABLE public.users
    ADD CONSTRAINT users_talent_network_visibility_two_states
    CHECK (talent_network_visibility IN ('off', 'anonymous')) NOT VALID;

UPDATE public.users
   SET talent_network_visibility = 'anonymous'
 WHERE talent_network_visibility = 'public';

-- squawk-ignore prefer-robust-stmts -- rerunnable as written: VALIDATE against an already-validated constraint is a no-op, not an error.
ALTER TABLE public.users
    VALIDATE CONSTRAINT users_talent_network_visibility_two_states;

ALTER TABLE public.users
    DROP CONSTRAINT IF EXISTS users_talent_network_visibility_check;
