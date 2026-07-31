-- Opt-in per-CV link tracing: the outbound links of a rendered CV point at our own redirect, which
-- records the click and forwards to the real destination. It answers one narrow question for the
-- candidate — was this CV opened at all — which separates "they read it and passed" from "it never
-- reached a human".
--
-- Off by default and settable only by the CV's owner over the cookie-authenticated update. It is
-- deliberately absent from PatchOps, the tailoring agent's path: consent to track a third party is
-- the candidate's to give, and an agent must not give it on their behalf.
--
-- ALTER TABLE takes ACCESS EXCLUSIVE. This project has already had DDL queue behind a long reader
-- and take the site down, so do not apply this inside the 03-07 UTC pg_dump window.
ALTER TABLE public.cvs ADD COLUMN tracer_links_enabled boolean NOT NULL DEFAULT false;

-- The last time a countable visitor opened a link in this CV. Denormalised from cv_link_clicks and
-- written in the same transaction as the click.
--
-- ListUserJobs already carries four correlated subqueries per row and is server-rendered; reading
-- the click history there would add a fifth, joining three tables, because only cvs.job_id connects
-- a click to an application. One indexed column instead.
--
-- "Countable" excludes both automated traffic and the owner's own clicks: the first thing a
-- candidate does after enabling this is download the PDF and click the link to check it works, and
-- reporting that back as "your CV was opened" would make the feature lie on first use.
--
-- Like user_jobs.followed_up_at (0059), this must NOT enter the application silence derivation. A
-- recruiter opening a CV is not a reply, and folding it into last_activity_at would clear the
-- silence badge at the moment it matters most.
ALTER TABLE public.cvs ADD COLUMN last_click_at timestamptz;

-- One row per (CV, position in the document, destination). The PDF is never stored — it is
-- re-rendered on every download — so "tokens are issued when the PDF is generated" means "on every
-- download", and the uniqueness key below is what makes re-minting idempotent. Without it three
-- downloads would produce three tokens for one link and scatter the counts across them.
CREATE TABLE public.cv_tracer_links (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cv_id            uuid        NOT NULL,
    -- What the visitor's URL carries: <company-slug>-<5 random chars>. The recruiter sees it on
    -- hover and in the address bar, where their own company's name reads less alarmingly than an
    -- opaque string.
    token            text        NOT NULL,
    -- Where in the document the link sits: 'header.links[1]', 'projects[0].link'.
    source_path      text        NOT NULL,
    destination_url  text        NOT NULL,
    -- sha256 of destination_url. In the unique index instead of the URL itself, which is an
    -- arbitrary-length string with query parameters; this keeps the key at a fixed 64 bytes.
    destination_hash text        NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT cv_tracer_links_token_key UNIQUE (token),
    -- source_path is part of the key on purpose: the same URL in the header and on a project gets
    -- two tokens, because "clicked the header link" and "clicked through to that project" are
    -- different events and merging them would erase the only interesting distinction.
    CONSTRAINT cv_tracer_links_position_key UNIQUE (cv_id, source_path, destination_hash)
);

-- Deleting a CV takes its tokens and every click on them. This is the right to erase one's own
-- data, implemented in the schema rather than in a delete path someone has to remember to write.
-- The accepted cost: links in already-sent PDFs die, and a recruiter opening a year-old CV gets a
-- dead link. The redirect answers 410 with an explanation rather than a bare 404.
ALTER TABLE ONLY public.cv_tracer_links
    ADD CONSTRAINT cv_tracer_links_cv_id_fkey
        FOREIGN KEY (cv_id) REFERENCES public.cvs(id) ON DELETE CASCADE;

-- The owner's per-CV panel lists every link of one CV.
CREATE INDEX cv_tracer_links_cv_id_idx ON public.cv_tracer_links USING btree (cv_id);

-- One row per click. Deliberately no raw IP and no bare hash of one: IPv4 has 4.3 billion
-- addresses, so an unsalted digest of an address is reversible by exhaustive search and would be
-- anonymisation in appearance only. visitor_hash is HMAC(configured salt, ip + user agent) and is
-- empty when no salt is configured — in which case the read surface reports no distinct-visitor
-- count at all rather than counting every such click as one visitor.
CREATE TABLE public.cv_link_clicks (
    id             bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tracer_link_id uuid        NOT NULL,
    clicked_at     timestamptz NOT NULL DEFAULT now(),
    -- Written once, at click time, and never recomputed. Were it evaluated on read, editing the
    -- detection markers would silently rewrite history: yesterday's twelve clicks would read as
    -- nine today with no new rows.
    --
    -- It cannot catch what matters most — corporate mail-security scanners fetch links with
    -- ordinary browser user agents — so counts are evidence a link was opened, never proof a person
    -- read the CV, and the surfaces must say so.
    is_likely_bot  boolean     NOT NULL DEFAULT false,
    -- The click came from the CV's own owner. The redirect is same-origin, so their session cookie
    -- rides along; such clicks are kept but excluded from every presented count.
    is_owner       boolean     NOT NULL DEFAULT false,
    device_type    text        NOT NULL DEFAULT 'unknown',
    os_family      text        NOT NULL DEFAULT 'unknown',
    ua_family      text        NOT NULL DEFAULT 'unknown',
    referrer_host  text        NOT NULL DEFAULT '',
    visitor_hash   text        NOT NULL DEFAULT ''
);

ALTER TABLE ONLY public.cv_link_clicks
    ADD CONSTRAINT cv_link_clicks_tracer_link_id_fkey
        FOREIGN KEY (tracer_link_id) REFERENCES public.cv_tracer_links(id) ON DELETE CASCADE;

-- Per-link aggregation for the panel, newest first.
CREATE INDEX cv_link_clicks_link_idx ON public.cv_link_clicks USING btree (tracer_link_id, clicked_at DESC);

-- The 180-day retention sweep in cmd/prune scans by age across every link.
CREATE INDEX cv_link_clicks_clicked_at_idx ON public.cv_link_clicks USING btree (clicked_at);
