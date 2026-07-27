-- Resolving a job page URL to the posting it is, for the browser extension's
-- /api/v1/jobs/find. The first tier reads the catalog identity (source, external_id) out
-- of the URL (internal/sources.RefFromURL), which only a handful of ATS URL shapes allow.
-- This is the second tier: the catalog already stores each posting's detail URL in
-- jobs.url, and for aggregators and most boards that is the very page the user is on.
--
-- normalize_job_url strips the noise two links to the same posting differ by: the scheme,
-- a leading www., the query string (our own outbound links carry ?utm_source=freehire.me),
-- the fragment, and trailing slashes. It exists as a function rather than an inline
-- expression so the index below and the query in internal/db/queries/jobs.sql share ONE
-- definition — an expression index is only used when the query's expression matches it
-- exactly, and a drift between two hand-copied expressions fails silently, degrading the
-- lookup into the sequential scan this endpoint was rewritten to escape.
--
-- IMMUTABLE (required to index it) holds: lower and regexp_replace are immutable, and the
-- rewrite depends on nothing but its argument. STRICT keeps a NULL url out of the index.
CREATE FUNCTION public.normalize_job_url(text) RETURNS text
    LANGUAGE sql IMMUTABLE PARALLEL SAFE STRICT
    AS $$
        SELECT regexp_replace(
                   regexp_replace(
                       regexp_replace(lower($1), '^https?://(www\.)?', ''),
                   '[?#].*$', ''),
               '/+$', '')
    $$;

-- Partial on the rows the lookup can answer with: a curated match card is for a vacancy
-- the user can still apply to, and a duplicate_of row is by definition not the one we
-- serve. That also keeps the index to the open catalogue rather than every posting we
-- have ever seen.
--
-- On a fresh initdb volume this plain CREATE INDEX is fine; on the live prod DB it is
-- applied manually as CREATE INDEX CONCURRENTLY.
CREATE INDEX jobs_normalized_url_idx
    ON public.jobs (public.normalize_job_url(url))
    WHERE closed_at IS NULL AND duplicate_of IS NULL;
