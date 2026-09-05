-- Rolling FORWARD again after 0135_pro_until_sources.down.sql. Read that file first.
--
-- This is not migrations/0135_pro_until_sources.sql run a second time, and reaching for that
-- file instead is the mistake this one exists to prevent: its ADD COLUMN would fail on the
-- source columns the rollback deliberately kept, and its backfill would overwrite them from
-- the derived value, re-splitting every account by stripe_customer_id and moving plans in
-- both directions.
--
-- The sources survived the rollback untouched, so restoring the derived column is all that
-- is left.
--
--   psql "$DATABASE_URL" -f deploy/rollback/0135_pro_until_sources.reapply.sql
--
-- FIRST settle anything the old binary wrote to the plain column while rolled back — the
-- query and the reasoning are at the foot of the down file. Whatever is still divergent when
-- this runs is discarded, because the derived column is computed from the sources alone.
BEGIN;

ALTER TABLE users DROP COLUMN pro_until;

ALTER TABLE users ADD COLUMN pro_until timestamp with time zone GENERATED ALWAYS AS (GREATEST(pro_until_stripe, pro_until_revenuecat, pro_until_granted)) STORED;

COMMIT;
