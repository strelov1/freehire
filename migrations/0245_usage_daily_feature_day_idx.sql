-- migrate: no-transaction
-- SumUsageDayForFeature runs before every support turn to hold the global daily ceiling.
-- usage_daily's key starts with user_id, so without this the read scans every account's
-- whole history. Not partial on feature = 'support': the query passes the feature as a
-- parameter, and a generic prepared plan cannot prove it matches a partial predicate.
CREATE INDEX CONCURRENTLY IF NOT EXISTS usage_daily_feature_day_idx
    ON public.usage_daily (feature, day);
