-- Local demo data for the /mentors screens. Not shipped: this file lives under .cache/,
-- which is untracked. Mentors are onboarded by hand in production, so without a seed the
-- directory is correctly, but uninformatively, empty.

INSERT INTO companies (slug, name) VALUES
  ('acme', 'Acme'),
  ('globex', 'Globex')
ON CONFLICT DO NOTHING;

INSERT INTO users (email) VALUES
  ('anna@example.test'),
  ('bo@example.test')
ON CONFLICT DO NOTHING;

-- Approved and unpaused, which is the only combination the public directory shows.
INSERT INTO mentors (
  user_id, company_slug, slug, display_name, headline, bio,
  topics, languages, timezone, session_duration_min,
  buffer_before_min, buffer_after_min, min_notice_min, horizon_days,
  meeting_url, status, paused
)
SELECT u.id, 'acme', 'anna-k', 'Anna K.', 'Staff Engineer',
       'Fifteen years across payments and platform teams. Happy to talk about levelling up, interview loops, and what the day actually looks like here.',
       ARRAY['career', 'system-design'], ARRAY['en', 'de'],
       'Europe/Berlin', 60, 0, 15, 120, 30,
       'https://meet.example.test/anna', 'approved', false
FROM users u WHERE u.email = 'anna@example.test'
ON CONFLICT DO NOTHING;

INSERT INTO mentors (
  user_id, company_slug, slug, display_name, headline, bio,
  topics, languages, timezone, session_duration_min,
  buffer_before_min, buffer_after_min, min_notice_min, horizon_days,
  meeting_url, status, paused
)
SELECT u.id, 'globex', 'bo-lindgren', 'Bo Lindgren', 'Engineering Manager',
       'I hire for this team. Ask me what a strong application looks like from the other side of the table.',
       ARRAY['career', 'hiring'], ARRAY['en', 'sv'],
       'Europe/Stockholm', 30, 0, 0, 60, 30,
       'https://meet.example.test/bo', 'approved', false
FROM users u WHERE u.email = 'bo@example.test'
ON CONFLICT DO NOTHING;

-- Weekly availability. `weekday` is Go's time.Weekday, so 0 is SUNDAY and 1..5 is
-- Monday..Friday — not the Monday-first order the calendar grid draws in.
INSERT INTO mentor_availability (mentor_id, weekday, start_time, end_time)
SELECT m.id, w, TIME '18:00', TIME '21:00'
FROM mentors m, generate_series(1, 5) AS w
WHERE m.slug = 'anna-k';

INSERT INTO mentor_availability (mentor_id, weekday, start_time, end_time)
SELECT m.id, w, TIME '09:00', TIME '12:00'
FROM mentors m, generate_series(2, 4) AS w
WHERE m.slug = 'bo-lindgren';
