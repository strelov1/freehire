-- name: InsertSignupAttribution :execrows
-- First report wins, and only for an account created in the last ten minutes: the
-- table records how an account ARRIVED, so a report from a years-old session would
-- be a made-up answer rather than an observation.
INSERT INTO user_signup_attribution (
    user_id, utm_source, utm_medium, utm_campaign, utm_content, utm_term,
    referrer_host, landing_path)
SELECT u.id, sqlc.narg('utm_source'), sqlc.narg('utm_medium'), sqlc.narg('utm_campaign'),
       sqlc.narg('utm_content'), sqlc.narg('utm_term'),
       sqlc.narg('referrer_host'), sqlc.narg('landing_path')
FROM users u
WHERE u.id = @user_id AND u.created_at > now() - interval '10 minutes'
ON CONFLICT (user_id) DO NOTHING;
