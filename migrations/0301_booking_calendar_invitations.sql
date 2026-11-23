-- Calendar invitation delivery is opt-in. Booking still holds the slot while Google
-- provisions Meet, and a durable event key lets retries reconcile a partial write.
ALTER TABLE users ADD COLUMN interview_calendar_invites_enabled boolean NOT NULL DEFAULT false;

ALTER TABLE booking_link_bookings
    ADD COLUMN calendar_event_key uuid,
    ADD COLUMN calendar_invitation_next_at timestamptz,
    ADD COLUMN calendar_invited_at timestamptz;
