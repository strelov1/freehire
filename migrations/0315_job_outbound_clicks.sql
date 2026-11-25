-- One row per click on a freehire redirect (/go/<slug>?src=<surface>) that sends a visitor to the
-- employer's site. It answers the employer's question "how many people did you send us for this
-- posting", which no surface could answer while each handed out the employer URL directly.
--
-- Same privacy contract as cv_link_clicks (0060): no raw IP and no bare hash of one. visitor_hash is
-- HMAC(configured salt, ip + user agent), empty when no salt is configured. Country is the
-- Cloudflare CF-IPCountry header, so no GeoIP database and no address is needed to have it.
--
-- Bots are stored and flagged, never dropped: the flag is written once at click time, so a later
-- edit of the bot markers cannot rewrite history, and the read side decides what to count.
--
-- No foreign key to jobs: an append-only log must neither slow nor be cascaded by a job delete
-- (moderation). The read side joins on job_id.
CREATE TABLE public.job_outbound_clicks (
    id            bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    job_id        bigint      NOT NULL,
    clicked_at    timestamptz NOT NULL DEFAULT now(),
    -- The surface that handed out the link: web, mcp, ojcp, assistant, email, telegram, api, or
    -- other for a value this deployment does not know.
    src           text        NOT NULL,
    is_likely_bot boolean     NOT NULL DEFAULT false,
    device_type   text        NOT NULL DEFAULT 'unknown',
    os_family     text        NOT NULL DEFAULT 'unknown',
    ua_family     text        NOT NULL DEFAULT 'unknown',
    referrer_host text        NOT NULL DEFAULT '',
    -- ISO 3166-1 alpha-2 as Cloudflare states it; empty when the header is absent. Cloudflare also
    -- sends XX (unknown) and T1 (Tor); they are kept as sent, not mapped.
    country       text        NOT NULL DEFAULT '',
    visitor_hash  text        NOT NULL DEFAULT ''
);

-- The employer dashboard reads one job's clicks over time.
CREATE INDEX job_outbound_clicks_job_idx ON public.job_outbound_clicks USING btree (job_id, clicked_at DESC);
