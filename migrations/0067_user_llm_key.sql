-- The per-user credential the LLM gateway knows this account by.
--
-- Every model call made on someone's behalf goes out on their own gateway key instead of
-- the one service credential, so the gateway can say what an account spent and on which
-- feature. The value is minted lazily on the account's first AI call and never shown to
-- them: it is infrastructure bookkeeping, not a feature they configure.
--
-- NULL means "not minted yet", which is the state every existing row starts in and the
-- state any row returns to if its key is ever revoked. There is no backfill: minting on
-- demand is the same code path a brand-new account takes, so a migration that pre-minted
-- for two hundred thousand rows would only add a way to be half-done.
--
-- UNIQUE is not ceremony. A key shared by two accounts would keep working — the calls
-- succeed, the spend simply lands on the wrong person — so the failure would be silent and
-- would corrupt exactly the numbers this column exists to produce. Postgres admits any
-- number of NULLs under a unique constraint, so "not minted yet" stays the common case.
--
-- Stored in plaintext because it must be presented on every call. Its whole power is to
-- spend inference against our own gateway, under whatever ceiling that gateway is
-- configured with, and revoking every key at once is one admin call plus a single UPDATE.
--
-- One nullable column: no table rewrite, no default to backfill, no lock of consequence
-- (contrast 0011). Additive and unread by the previous binary, so the deploy order does not
-- matter and a rollback leaves the column harmlessly behind.
ALTER TABLE public.users
    ADD COLUMN llm_key text;

ALTER TABLE public.users
    ADD CONSTRAINT users_llm_key_key UNIQUE (llm_key);
