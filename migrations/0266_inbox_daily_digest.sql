-- Apply after 0256 and before deploying inbox-digest readers. Existing accounts stay opted out.
ALTER TABLE notification_settings
 ADD COLUMN inbox_digest_enabled boolean NOT NULL DEFAULT false,
 ADD COLUMN inbox_digest_enabled_at timestamptz;

-- Claim before dispatch. A day with an uncertain outcome stays claimed until the next day.
-- Only confirmed sends retire their covered message ids; no mail content is stored here.
CREATE TABLE inbox_digest_deliveries (
 user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 local_day date NOT NULL,
 email_ids bigint[] NOT NULL,
 claimed_at timestamptz NOT NULL,
 sent_at timestamptz,
 PRIMARY KEY (user_id, local_day)
);

-- Match deliverywindow's UTC fallback without letting an invalid stored zone abort a batch.
CREATE FUNCTION inbox_digest_timezone(zone_name text) RETURNS text
LANGUAGE sql STABLE AS $$
 SELECT coalesce((SELECT name FROM pg_timezone_names WHERE name = zone_name), 'UTC');
$$;
