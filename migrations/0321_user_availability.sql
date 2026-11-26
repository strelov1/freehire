-- One availability calendar per user for mentorship, buddy mocks and booking links: weekly
-- rules and dated overrides in the shape mentor_availability (0145) and
-- buddy_mock_availability (0215) already have, plus one zone and the booking-link session
-- parameters. The old tables stay until a follow-up migration drops them after deploy.
CREATE TABLE IF NOT EXISTS user_availability (
    id         bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- A day of the week, 0 through 6.
    -- squawk-ignore prefer-bigint-over-smallint
    weekday    smallint,
    on_date    date,
    start_time time        NOT NULL,
    end_time   time        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT user_availability_shape_check
        CHECK ((weekday IS NULL) <> (on_date IS NULL)),
    CONSTRAINT user_availability_weekday_check
        CHECK (weekday IS NULL OR weekday BETWEEN 0 AND 6),
    CONSTRAINT user_availability_span_check
        CHECK (end_time >= start_time AND (on_date IS NOT NULL OR end_time > start_time))
);

CREATE INDEX IF NOT EXISTS user_availability_user_idx ON user_availability (user_id);

-- No row means "never saved": slots fall back to each product's own zone, and a booking
-- link to its default week. Times in user_availability resolve through timezone.
CREATE TABLE IF NOT EXISTS user_availability_settings (
    user_id              bigint      PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    timezone             text        NOT NULL,
    -- CHECK bounds it to 15-60.
    -- squawk-ignore prefer-bigint-over-int
    booking_duration_min integer     NOT NULL DEFAULT 30,
    -- CHECK bounds it to 0-10080.
    -- squawk-ignore prefer-bigint-over-int
    booking_notice_min   integer     NOT NULL DEFAULT 720,
    updated_at           timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT user_availability_settings_duration_check
        CHECK (booking_duration_min IN (15, 30, 45, 60)),
    CONSTRAINT user_availability_settings_notice_check
        CHECK (booking_notice_min BETWEEN 0 AND 10080)
);

-- Mentor rows are kept; buddy rows only for users who have no mentor rows, so a user who
-- is both does not get their two weeks stacked.
INSERT INTO user_availability (user_id, weekday, on_date, start_time, end_time, created_at)
SELECT m.user_id, a.weekday, a.on_date, a.start_time, a.end_time, a.created_at
  FROM mentor_availability a
  JOIN mentors m ON m.id = a.mentor_id
 ORDER BY a.id;

INSERT INTO user_availability (user_id, weekday, on_date, start_time, end_time, created_at)
SELECT b.user_id, b.weekday, b.on_date, b.start_time, b.end_time, b.created_at
  FROM buddy_mock_availability b
 WHERE NOT EXISTS (
     SELECT 1 FROM mentor_availability a JOIN mentors m ON m.id = a.mentor_id
      WHERE m.user_id = b.user_id)
 ORDER BY b.id;

INSERT INTO user_availability_settings (user_id, timezone)
SELECT m.user_id, m.timezone FROM mentors m
 WHERE EXISTS (SELECT 1 FROM mentor_availability a WHERE a.mentor_id = m.id)
ON CONFLICT DO NOTHING;

-- The rest (buddy rows only) keep the zone their rows were written in.
INSERT INTO user_availability_settings (user_id, timezone)
SELECT s.user_id, s.timezone FROM buddy_mock_settings s
 WHERE EXISTS (SELECT 1 FROM user_availability u WHERE u.user_id = s.user_id)
ON CONFLICT DO NOTHING;
