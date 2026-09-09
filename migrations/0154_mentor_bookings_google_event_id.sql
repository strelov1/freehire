-- google_event_id is the Google Calendar event this booking minted, when its mentor has
-- connected calendar.events and generation succeeded. Empty for every booking made
-- without that connection, or when generation failed — see mentor-google-meet-link.
--
-- NOT NULL DEFAULT '' is additive on Postgres 11+ (a constant default needs no table
-- rewrite), matching meeting_url's own NOT NULL-with-empty-string convention rather than
-- a nullable column: "no event" and "not yet known" are the same fact here, so there is
-- no third state worth a NULL.
ALTER TABLE mentor_bookings ADD COLUMN google_event_id text NOT NULL DEFAULT '';
