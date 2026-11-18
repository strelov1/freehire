-- How many API requests each account made per UTC day, for the account's own API page.
--
-- Counted by the API process (internal/identity/apiusage) from the attribution the rate
-- gateway forwards (X-FH-Auth-User), only for requests carrying a Bearer credential, so
-- browsing the site signed in never counts. A request the gateway refused with 429 never
-- reaches the API and is not here. Counts are buffered in memory and added in batches, so
-- a crash can lose up to one flush interval.
--
-- `errors` is the part of `requests` answered with a 4xx or 5xx.
CREATE TABLE api_usage_daily (
    user_id bigint NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    day date NOT NULL,
    requests bigint NOT NULL DEFAULT 0,
    errors bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, day)
);
