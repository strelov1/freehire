-- Reverse of migrations/0135_pro_until_sources.sql. See the add-store-purchases change.
--
-- IT IS NOT IN migrations/ AND MUST NOT BE MOVED THERE. The runner is forward-only and
-- initdb executes every *.sql under migrations/ in name order (docker-compose mounts the
-- directory straight into docker-entrypoint-initdb.d), so a down file living beside its up
-- file would be applied to every fresh volume immediately after it — undoing the migration
-- on install, silently, and only on new environments.
--
-- Why it exists: rolling the application back past this change means redeploying a binary
-- whose SetProUntil assigns users.pro_until, and that statement fails while the column is
-- generated. Schema-first deploys are safe in one direction only, and this file is the
-- other direction. Writing it now, against a schema we can still test it on, is the
-- difference between a rollback and an outage spent composing SQL from memory.
--
-- Run it with psql on the host, before redeploying the older binary:
--   psql "$DATABASE_URL" -f deploy/rollback/0135_pro_until_sources.down.sql
--
-- IT KEEPS THE THREE SOURCE COLUMNS, and that is the whole design of this file rather than
-- an oversight. Dropping them would be tidier and is wrong: which origin conferred a plan
-- is not reconstructible afterwards, and re-applying 0135 over the wreckage re-splits by
-- stripe_customer_id, which silently moves money in both directions. A store subscriber
-- with no Stripe customer becomes an UNREVOCABLE manual grant — no refund, cancellation or
-- lapse can ever take their plan away again. A Stripe customer holding a longer manual
-- grant has that grant turned into a Stripe value, and the next sync shortens it to
-- whatever Stripe says. Neither announces itself.
--
-- The old binary neither reads nor writes the source columns, so leaving them costs it
-- nothing. They are inert until something rolls forward again.
BEGIN;

-- GREATEST is read into the plain column BEFORE the generated one goes away, so no account
-- gains or loses a day across the rollback.
ALTER TABLE users DROP COLUMN pro_until;

ALTER TABLE users ADD COLUMN pro_until timestamp with time zone;

UPDATE users
SET pro_until = GREATEST(pro_until_stripe, pro_until_revenuecat, pro_until_granted)
WHERE pro_until_stripe IS NOT NULL
   OR pro_until_revenuecat IS NOT NULL
   OR pro_until_granted IS NOT NULL;

COMMIT;

-- ROLLING FORWARD AGAIN IS NOT "RE-RUN 0135". 0135 would fail on its ADD COLUMN — the
-- source columns are still here — and its backfill would overwrite them from the derived
-- value. Rolling forward is these two statements and nothing else:
--
--   BEGIN;
--   ALTER TABLE users DROP COLUMN pro_until;
--   ALTER TABLE users ADD COLUMN pro_until timestamp with time zone
--       GENERATED ALWAYS AS (GREATEST(pro_until_stripe, pro_until_revenuecat, pro_until_granted)) STORED;
--   COMMIT;
--
-- ONE THING NEEDS A HUMAN, and only if the old binary ran for long enough to write anything.
-- While rolled back, its Stripe sync writes the plain pro_until and knows nothing of the
-- sources, so those writes are discarded the moment the column becomes derived again. Before
-- rolling forward, look at what diverged:
--
--   SELECT id, pro_until, pro_until_stripe, pro_until_revenuecat, pro_until_granted
--   FROM users
--   WHERE pro_until IS DISTINCT FROM GREATEST(pro_until_stripe, pro_until_revenuecat, pro_until_granted);
--
-- There is no correct automatic fold for that set, which is why this file does not attempt
-- one: a value the old Stripe sync wrote belongs in pro_until_stripe, a value it CLEARED
-- means a cancellation that the stale source column would otherwise keep alive, and neither
-- is distinguishable from a hand-set value without knowing what ran when. At the measured
-- 1397 accounts on prod this query returns a handful of rows a person can settle in minutes.
