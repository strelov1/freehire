-- Saved-search email digests default to once a day at 09:00 local time.
--
-- SES shut the sending account on 2026-11-06 for a complaint rate over 1%. The hourly
-- interval digest (migration 0233) was most of the volume: ~16k mails a day for ~2.3k
-- people with an email alert. The owner moved everybody to one digest a day.
--
-- A daily digest needs a time: deliverywindow.DigestDue never fires without one. So
-- the time gets the same default, and the rows without one are filled in. The
-- queries COALESCE both columns to the same values for accounts with no row.
-- notification_settings holds ~1k rows, so the UPDATEs are one short transaction.

ALTER TABLE notification_settings
    ALTER COLUMN digest_frequency SET DEFAULT 'daily';

ALTER TABLE notification_settings
    ALTER COLUMN digest_time SET DEFAULT '09:00';

UPDATE notification_settings
SET digest_time = '09:00'
WHERE digest_time IS NULL;

UPDATE notification_settings
SET digest_frequency = 'daily'
WHERE digest_frequency IN ('interval', 'instant');
