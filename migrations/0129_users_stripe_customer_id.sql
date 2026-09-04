-- The payment provider's identifier for this account. See the add-pro-subscription change.
--
-- It exists because the reconciler has to go the OTHER WAY round from the webhook. A
-- delivery names the customer and we look up the user; a scheduled re-check starts from a
-- user and has to name the customer. Without a stored mapping that second direction has no
-- answer short of scanning the provider's customer list.
--
-- Nullable, and NULL is the ordinary state: it is set the first time an account transacts,
-- and the overwhelming majority of accounts never will. Every read treats NULL as "has
-- never bought anything", which is the same answer the provider gives for an id it does
-- not know.
--
-- NOT the source of truth about the subscription — that stays with the provider, and
-- users.pro_until stays the one derived fact. This column only says WHO to ask about.
--
-- UNIQUE, because two accounts sharing one customer would mean one payment silently
-- deciding two people's plans, and the constraint is cheaper than the incident. A partial
-- index so the millions of NULLs cost nothing: in Postgres NULLs do not collide under a
-- plain UNIQUE either, but saying so in the index keeps it off every row that will never
-- have a value.
ALTER TABLE users ADD COLUMN stripe_customer_id text;

-- squawk-ignore require-concurrent-index-creation -- users held 1397 rows when this was written (measured on prod, not estimated), so the build is milliseconds and blocking writes for it is cheaper than CONCURRENTLY's failure mode: it cannot run in a transaction, it waits on unrelated open transactions, and a 55P03 leaves an INVALID index behind that `IF NOT EXISTS` then skips forever.
CREATE UNIQUE INDEX users_stripe_customer_id_uniq
    ON public.users (stripe_customer_id) WHERE stripe_customer_id IS NOT NULL;
