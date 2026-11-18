-- The company page's "Subscribe to updates" button is gone: following a company is now the
-- watchlist (0248), and its alert is the one saved search `watchlist=true`. This moves every
-- follow the old button made into that shape, so nobody loses an alert and nobody gets the
-- same posting twice from both.
--
-- A follow is a saved search whose query is exactly one company (`company_slug=<slug>`, the
-- only query that button ever wrote) WITH a subscription. One without a subscription is a
-- bookmark the person saved from the filters, not a follow, and is left alone.
--
-- The moved subscriptions start now rather than at their old start_at, so the first digest
-- cannot replay a backlog; the cost is at most one interval of postings between the last
-- old digest and this migration. Matches still pending on the old subscriptions are dropped
-- with them by the cascade, for the same reason.

CREATE TEMP TABLE company_follows AS
SELECT ss.id AS saved_search_id, ss.user_id,
       substring(ss.query FROM '^company_slug=(.+)$') AS company_slug
FROM saved_searches ss
WHERE ss.query ~ '^company_slug=[^&,%]+$'
  AND EXISTS (SELECT 1 FROM subscriptions s WHERE s.saved_search_id = ss.id);

-- The company joins the list. The 5,000 cap is not checked here: a follow was one click per
-- company, so nobody can arrive near it this way.
INSERT INTO company_watchlist (user_id, company_slug)
SELECT DISTINCT user_id, company_slug FROM company_follows
ON CONFLICT DO NOTHING;

-- One watchlist alert per follower, unless they already saved one. The name is the one the
-- web app gives that query (alertName); a person who already used it for another search
-- gets a suffixed one instead of a unique-name violation.
INSERT INTO saved_searches (user_id, name, query)
SELECT f.user_id,
       CASE WHEN EXISTS (SELECT 1 FROM saved_searches n
                         WHERE n.user_id = f.user_id AND n.name = 'Companies I follow')
            THEN 'Companies I follow (watchlist)' ELSE 'Companies I follow' END,
       'watchlist=true'
FROM (SELECT DISTINCT user_id FROM company_follows) f
WHERE NOT EXISTS (SELECT 1 FROM saved_searches w
                  WHERE w.user_id = f.user_id AND w.query = 'watchlist=true');

-- Each channel a person followed any company on, once, on that alert. A paused follow stays
-- paused only when every follow on that channel was paused.
INSERT INTO subscriptions (user_id, saved_search_id, channel, destination, active)
SELECT f.user_id, w.id, s.channel, max(s.destination), bool_or(s.active)
FROM company_follows f
JOIN subscriptions s ON s.saved_search_id = f.saved_search_id
JOIN LATERAL (SELECT id FROM saved_searches
              WHERE user_id = f.user_id AND query = 'watchlist=true'
              ORDER BY id LIMIT 1) w ON true
GROUP BY f.user_id, w.id, s.channel
ON CONFLICT (saved_search_id, channel) DO NOTHING;

DELETE FROM subscriptions s
USING company_follows f
WHERE s.saved_search_id = f.saved_search_id;

-- The saved search goes too when the old button made it (it named it after the company, and
-- removed it on unfollow by the same rule); one the person named themselves stays as theirs.
DELETE FROM saved_searches ss
USING company_follows f
JOIN companies c ON c.slug = f.company_slug
WHERE ss.id = f.saved_search_id AND ss.name = c.name;

-- squawk-ignore ban-drop-table -- the session temp table this migration created above, nothing reads it
DROP TABLE company_follows;
