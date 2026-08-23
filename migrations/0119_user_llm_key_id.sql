-- The gateway's OWN identifier for the credential in users.llm_key.
--
-- 0067 stored the secret and nothing else, because the gateway it was written for
-- addressed a key by its secret: block and delete both took `{"key": "sk-..."}`. The
-- gateway being migrated to addresses a key by an opaque id instead, and only by that —
-- a PUT or DELETE aimed at the secret answers 404. Without this column an account's
-- credential can be minted and spent but never blocked, which is precisely what account
-- deletion needs to do.
--
-- Two columns rather than one composite value. They are written in the same statement
-- and read together, so packing them into one string would save a column and cost every
-- reader a parse, plus a way to be wrong about the separator.
--
-- Nullable with no backfill, for the same reason 0067 was: an id cannot be derived for a
-- credential minted by the old gateway. Those rows keep working for inference — the
-- secret is all a model call needs — and the first time one of them is refused, the
-- existing forget-and-re-mint path replaces it with a pair. Nothing has to migrate; the
-- population drains on its own.
--
-- UNIQUE for the same reason the secret is: two accounts sharing an id would block or
-- delete each other's credential, and the wrongness would surface as somebody else's
-- account losing AI, far from the cause.
--
-- The UNIQUE constraint builds an index under ACCESS EXCLUSIVE, which is the one thing
-- here that blocks. It is the same shape 0067 took for the secret and for the same table,
-- and `users` is small enough that the scan is milliseconds — but it does queue behind and
-- ahead of anything else touching the table, so run it like any other DDL: not while the
-- nightly dump holds its locks. If `users` ever grows past that assumption, the escape is
-- CREATE UNIQUE INDEX CONCURRENTLY followed by ADD CONSTRAINT ... USING INDEX, which needs
-- its own migration because CONCURRENTLY cannot run inside a transaction.
--
-- One nullable column: no rewrite, no default. Additive and
-- unread by the previous binary, so deploy order does not matter and a rollback leaves
-- the column harmlessly behind.
ALTER TABLE public.users
    ADD COLUMN llm_key_id text;

ALTER TABLE public.users
    ADD CONSTRAINT users_llm_key_id_key UNIQUE (llm_key_id);
