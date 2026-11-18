-- Where an account came from, written once when it registers.
--
-- Google Analytics only sees the visitors who accepted its cookies, so on a day of
-- 644 sign-ups it saw 228. This table is the complete count: the browser reports the
-- visit that led to the sign-up (campaign tags, the referring HOST, the landing path)
-- and the server keeps the first report per account. Nothing here identifies a person
-- beyond the account it belongs to, and nothing here is a cookie id.
--
-- referrer_host and landing_path are deliberately not URLs: a full referrer or landing
-- URL can carry a token or an address in its query string.
--
-- Apply before deploying the handler that writes it.
CREATE TABLE IF NOT EXISTS user_signup_attribution (
    user_id       bigint      PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    utm_source    text,
    utm_medium    text,
    utm_campaign  text,
    utm_content   text,
    utm_term      text,
    referrer_host text,
    landing_path  text,
    created_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT user_signup_attribution_bounds CHECK (
        char_length(coalesce(utm_source, ''))    <= 100 AND
        char_length(coalesce(utm_medium, ''))    <= 100 AND
        char_length(coalesce(utm_campaign, ''))  <= 200 AND
        char_length(coalesce(utm_content, ''))   <= 200 AND
        char_length(coalesce(utm_term, ''))      <= 200 AND
        char_length(coalesce(referrer_host, '')) <= 255 AND
        char_length(coalesce(landing_path, ''))  <= 300)
);

COMMENT ON TABLE user_signup_attribution IS
    'First-touch source of an account, reported by the browser right after sign-up. '
    'One row per account; the first report wins. Independent of analytics consent.';
