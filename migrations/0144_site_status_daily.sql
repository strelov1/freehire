-- One row per calendar day, holding the WORST site-status severity observed
-- that day (see internal/api/handler.deriveSiteStatus and the site-status
-- sampler in cmd/server): 0=operational, 1=degraded, 2=down. Fed by an
-- upsert (`GREATEST(existing, new)`) from a ticker inside cmd/server every
-- few minutes, not by a batch recompute — a day's value can only move to a
-- worse severity within that same day, never back down, so a brief outage
-- stays visible in that day's tile even after the site recovers.
--
-- The public GET /api/v1/status endpoint reads the trailing 90 days for the
-- /status page's history strip. A day with no row here is reported as
-- having no data, never backfilled as "operational" — this table only ever
-- gets a row when a sample actually ran.
CREATE TABLE public.site_status_daily (
    day            date        PRIMARY KEY,
    -- squawk-ignore prefer-bigint-over-smallint
    worst_severity smallint    NOT NULL, -- only ever 0/1/2 (see the table comment above);
                                          -- bigint buys no headroom a fixed 3-value column can use.
    updated_at     timestamptz NOT NULL DEFAULT now()
);
