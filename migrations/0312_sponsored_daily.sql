-- Per-campaign per-UTC-day counters. spent_cents is what a daily cap is checked against.
CREATE TABLE sponsored_daily (
    campaign_id    bigint NOT NULL REFERENCES sponsored_campaigns (id) ON DELETE CASCADE,
    day            date   NOT NULL,
    impressions    bigint NOT NULL DEFAULT 0,
    valid_clicks   bigint NOT NULL DEFAULT 0,
    invalid_clicks bigint NOT NULL DEFAULT 0,
    spent_cents    bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (campaign_id, day)
);
