-- Apply before deploying weekly application digest readers. Existing accounts remain opted out.
ALTER TABLE notification_settings
 ADD COLUMN application_digest_enabled boolean NOT NULL DEFAULT false,
 ADD COLUMN application_digest_enabled_at timestamptz;

CREATE TABLE application_digest_deliveries (
 user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 local_week date NOT NULL,
 claimed_at timestamptz NOT NULL,
 sent_at timestamptz,
 PRIMARY KEY (user_id, local_week)
);
