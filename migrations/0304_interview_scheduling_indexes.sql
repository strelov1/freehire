-- migrate: no-transaction
-- These tables already serve live reads and writes.
CREATE INDEX CONCURRENTLY IF NOT EXISTS users_interview_scheduling_enabled_idx
    ON users (id) WHERE interview_scheduling_enabled_at IS NOT NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS booking_link_bookings_calendar_due_idx
    ON booking_link_bookings (calendar_invitation_next_at, id)
    WHERE status = 'confirmed' AND calendar_invitation_next_at IS NOT NULL;
