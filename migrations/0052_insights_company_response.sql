-- Per-company application response rate: how many applications we can OBSERVE the
-- outcome of, and how many of those were answered.
--
-- A scalar table beside insights_company_growth rather than columns on it. Both are
-- per-company scalars rebuilt by cmd/rollup-company in one transaction, but they
-- answer unrelated questions — one is how fast a company is hiring, the other is how
-- it treats the people who apply — and a company can look excellent on either while
-- looking terrible on the other. Folding them together would invite a reader to
-- treat one as evidence for the other.
--
-- This is the measure a 2026-07-21 investigation identified as the ONLY unconfounded
-- company-level signal, after ruling out posting age and never-closing as artifacts
-- of company type and of our own ingest history. It is expected to be absent for
-- nearly every company until the sample matures: that absence is the correct answer,
-- not an unfinished implementation.
--
-- Applied to a fresh volume by initdb after 0051; on an existing prod volume run this
-- manually (SET ROLE hire) BEFORE deploying code that reads it.

CREATE TABLE IF NOT EXISTS insights_company_response (
    company_slug text    NOT NULL PRIMARY KEY,
    -- Applications whose outcome is OBSERVABLE: the applicant has a connected
    -- mailbox, so a reply would have been seen. Applications from people we cannot
    -- observe are excluded from BOTH sides, because counting them in the denominator
    -- would report our own blind spot as the employer's silence.
    applications integer NOT NULL DEFAULT 0,
    -- Of those, the ones that received something back.
    answered     integer NOT NULL DEFAULT 0
);
