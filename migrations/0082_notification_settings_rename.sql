-- Widen reminder_settings into notification_settings: the same account-level
-- enabled+channels gate now governs saved-job reminders AND the two new
-- lifecycle nudges (follow-up, interview-prep) added by application_nudges
-- (0083). default_delay_days is dropped — the delay is no longer configurable,
-- internal/reminder uses a fixed constant instead. See the
-- centralize-lifecycle-notifications change.
--
-- Never edit 0034: this is a new migration on top of it, not a rewrite.

ALTER TABLE public.reminder_settings RENAME TO notification_settings;

ALTER TABLE public.notification_settings DROP COLUMN default_delay_days;
