-- Which COMPANY pages have been announced to which external search engine.
--
-- A sibling of job_search_pings rather than a column on it, because the two ledgers key
-- on different things — a job id and a company slug — and a shared table would have to
-- make one of them nullable, which is a table that cannot state its own invariant.
--
-- The company page is the asset this exists for, and the evidence is Bing's, not a
-- guess. Measured 2026-09-15 through the Bing Webmaster API: 255,038 pages in its index
-- and climbing ~6k/day, its highest-impression pages are /companies/<slug> (laserfocus,
-- astra-tech-labs, truebiz, read-bean), and the queries behind them are of one shape —
-- "<company name> careers". Google's own query data from early August says the same
-- thing ("princess cruises careers", "techno brain careers"). Bingbot votes with its
-- crawl too: 6,275 company pages against 3,679 job pages in two days.
--
-- The reason is structural. A job page's text belongs to the employer and exists in a
-- dozen other copies; a company page is OUR assembly — every open role of one employer
-- in one place — which the employer often does not publish anywhere.
--
-- NO kind column, unlike job_search_pings. A job has two events worth announcing
-- (it appears, it closes); a company page has one (it exists). It gains and loses
-- postings continuously and its URL never dies of it — a company whose jobs all close
-- keeps a 200 page, verified — so there is no closure to announce.
CREATE TABLE public.company_search_pings (
    company_slug text NOT NULL,
    engine       text NOT NULL,
    pinged_at    timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT company_search_pings_pkey PRIMARY KEY (company_slug, engine)
);

ALTER TABLE ONLY public.company_search_pings
    ADD CONSTRAINT company_search_pings_company_slug_fkey
    FOREIGN KEY (company_slug) REFERENCES public.companies(slug) ON DELETE CASCADE;

-- "What has this engine been told, and when" — the same reporting direction
-- job_search_pings_engine_pinged_at_idx serves, and the same reason: a bounded budget
-- is only visible if it can be counted. The anti-join that picks candidates is served
-- by the primary key above.
CREATE INDEX company_search_pings_engine_pinged_at_idx
    ON public.company_search_pings (engine, pinged_at DESC);
