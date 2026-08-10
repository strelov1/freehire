-- rate_limits backed PGThrottler, replaced by the Redis-backed shared rate
-- limiter (internal/ratelimit). Nothing reads or writes this table anymore.
DROP TABLE IF EXISTS rate_limits;
