-- The covering index the company sitemap pages by: slug order, the lastmod it
-- emits, and the hiring predicate it filters on — so both sitemap reads are
-- index-only and never touch the companies heap.
--
-- The sitemap's two queries (ListCompanySitemap, CompanySitemapBoundaries) run on
-- every crawler fetch of /sitemap.xml and each company chunk. Measured on prod
-- (2026-07-29, 320k companies, 7.7 GB heap): the boundary walk takes 2.8 s and an
-- unfiltered 50k-row chunk 0.9 s warm — but both swing past nginx's 60 s ceiling
-- while an ingest run evicts the buffer cache, and three of three probes 504'd
-- inside one ten-minute window. Chunk size alone doesn't fix that: adding the
-- `job_count > 0` scope the sitemap wants made a *smaller* 20k chunk slower (8.4 s),
-- because companies_pkey carries neither the predicate nor updated_at, so every
-- candidate row went to the heap twice over.
--
-- With this index a chunk is a bounded index-only range scan, and the boundary walk
-- reads ~209k narrow index tuples instead of heap-checking 306k rows. Partial on the
-- same `job_count > 0` scope as the /companies catalog (see 0023), which is also
-- what the sitemap now lists — so it stays proportional to the hiring set.
--
-- release.sh applies migrations before restarting the new color, so this builds on
-- deploy: a plain CREATE INDEX holds a SHARE lock on companies (blocking upserts,
-- not reads) for the few seconds a 209k-row partial index takes. If a deploy ever
-- can't afford even that, build it CONCURRENTLY beforehand — psql -f from a file
-- under systemd-run, never `psql -c` over ssh — and IF NOT EXISTS makes the
-- migration a no-op.
CREATE INDEX IF NOT EXISTS companies_sitemap_hiring_idx
    ON public.companies (slug) INCLUDE (updated_at)
    WHERE job_count > 0;
