-- Optional mail goes only to addresses the person proved they own and to people who
-- used freehire in the last 180 days.
--
-- SES shut the sending account on 2026-11-06 for complaints, and its Trust & Safety
-- review asked us to stop mailing addresses nobody confirmed and people we have not
-- heard from in six months. Neither was enforced: digests, reminders and nudges read
-- users.email with no check at all, and nothing recorded when an account was last used.
--
-- last_active_at is stamped by the session check (GetUserTokenVersion) at most once a
-- day. It starts from the newest trace an account already left: sign-up, an API key or
-- connector used, an assistant session, a CV edit, an application, a saved search, a
-- push device seen. users holds ~21.5k rows, so the backfill is one short transaction.

-- A new account is active the moment it exists; the backfill below replaces the
-- migration time on every existing row.
ALTER TABLE users ADD COLUMN last_active_at timestamptz NOT NULL DEFAULT now();

UPDATE users u SET last_active_at = greatest(
    u.created_at,
    (SELECT max(last_used_at) FROM api_keys k WHERE k.user_id = u.id),
    (SELECT max(last_used_at) FROM oauth_grants g WHERE g.user_id = u.id),
    (SELECT max(updated_at) FROM assistant_sessions a WHERE a.user_id = u.id),
    (SELECT max(updated_at) FROM cvs c WHERE c.user_id = u.id),
    (SELECT max(created_at) FROM applications p WHERE p.user_id = u.id),
    (SELECT max(updated_at) FROM saved_searches s WHERE s.user_id = u.id),
    (SELECT max(last_seen_at) FROM user_push_tokens t WHERE t.user_id = u.id));

-- The one rule for every optional mail: the account's address when it may be mailed,
-- '' when it may not. An empty address is what every sender already reads as "the email
-- channel is unavailable" and soft-skips, so Telegram and push keep working. Essential
-- mail (codes, resets, a booked session) does not go through it.
CREATE FUNCTION mail_recipient(email text, email_verified boolean, last_active_at timestamptz) RETURNS text
LANGUAGE sql STABLE PARALLEL SAFE
AS $$ SELECT CASE WHEN email_verified AND last_active_at > now() - interval '180 days' THEN email ELSE '' END $$;
