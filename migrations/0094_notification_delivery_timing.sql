-- Notification delivery timing: an account-wide timezone, a saved-search digest
-- frequency (instant/daily), and a quiet-hours window that defers delivery
-- across all three notification engines. All additive, nullable/defaulted, no
-- backfill needed to preserve today's behavior for every existing account.
--
-- digest_frequency defaults to 'instant' (today's only behavior); digest_time
-- and the quiet-hours pair stay NULL (off) until explicitly configured.
-- last_digest_sent_at tracks, per subscription, the last local calendar day a
-- daily-mode digest was sent — see internal/deliverywindow.DigestDue.

ALTER TABLE public.users ADD COLUMN timezone text;

ALTER TABLE public.notification_settings
    ADD COLUMN digest_frequency text NOT NULL DEFAULT 'instant',
    ADD COLUMN digest_time time,
    ADD COLUMN quiet_hours_start time,
    ADD COLUMN quiet_hours_end time,
    ADD CONSTRAINT notification_settings_digest_frequency_check
        CHECK (digest_frequency = ANY (ARRAY['instant'::text, 'daily'::text]));

ALTER TABLE public.subscriptions ADD COLUMN last_digest_sent_at timestamptz;
