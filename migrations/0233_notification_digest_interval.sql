-- Saved-search alerts gain a third delivery frequency, 'interval': at most one digest
-- per subscription every digest_interval_hours hours (1-23; a full day is 'daily',
-- which also picks the hour). It becomes the DEFAULT, at one hour, for every account
-- that has not chosen 'daily' — including the accounts that explicitly stored
-- 'instant', which is the product decision this migration carries out.
--
-- Why: 'instant' sent a digest on every five-minute notify pass that found anything
-- new. Measured on prod 2026-11-04 over seven days: 113k subscription digests to 1.6k
-- accounts, median 2.4 a day, p90 ~32 a day, the busiest account 141 a day, and 96%
-- of subscribers on 'instant' — 1,558 of them with no settings row at all, so the
-- default IS the product for nearly everyone. Users reported it as too many.
--
-- 'instant' stays a choosable value; it simply stops being anybody's default.
--
-- The CHECK list is rewritten in full (Postgres has no ALTER CONSTRAINT for a CHECK's
-- expression). The new constraint is strictly wider than the one it replaces, so no
-- existing row can violate it. digest_interval_hours is NOT NULL with a constant
-- default, a metadata-only change on PG11+, and is read only in 'interval' mode.
--
-- An old binary reads 'interval' as 'instant' (its worker treats anything other than
-- exactly 'daily' that way), so a rollback degrades to the previous behaviour rather
-- than breaking delivery; its settings API would refuse to SAVE the value until the
-- new binary is back. The UPDATE cannot be undone precisely — an account that later
-- chooses 'interval' itself is indistinguishable from one moved here.

-- The explicit transaction closes the window between DROP and ADD (see 0232).
-- squawk-ignore transaction-nesting
BEGIN;

ALTER TABLE public.notification_settings
    DROP CONSTRAINT IF EXISTS notification_settings_digest_frequency_check;

ALTER TABLE public.notification_settings
    -- squawk-ignore constraint-missing-not-valid -- strictly wider than the constraint it replaces, so the validating scan cannot fail
    ADD CONSTRAINT notification_settings_digest_frequency_check
    CHECK (digest_frequency = ANY (ARRAY['instant'::text, 'daily'::text, 'interval'::text]));

ALTER TABLE public.notification_settings
    -- squawk-ignore prefer-bigint-over-smallint -- an hour count the CHECK below bounds to 1-23; it cannot approach any integer limit
    ADD COLUMN digest_interval_hours smallint NOT NULL DEFAULT 1;

ALTER TABLE public.notification_settings
    -- squawk-ignore constraint-missing-not-valid -- the column was added a statement ago with a constant default inside the bounds, so every row satisfies it
    ADD CONSTRAINT notification_settings_digest_interval_hours_check
    CHECK (digest_interval_hours BETWEEN 1 AND 23);

ALTER TABLE public.notification_settings
    ALTER COLUMN digest_frequency SET DEFAULT 'interval';

-- A few hundred rows (prod 2026-11-04: 269 accounts with a stored 'instant').
UPDATE public.notification_settings
SET digest_frequency = 'interval', digest_interval_hours = 1
WHERE digest_frequency = 'instant';

-- squawk-ignore transaction-nesting
COMMIT;
