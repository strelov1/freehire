-- migrate: no-transaction
--
-- normalize_job_url keeps the query parameters that name a posting (freehire#4821).
--
-- 0042 dropped the WHOLE query string, on the theory that a query only ever carries
-- tracking tags. On a site where the query is what names the posting that collapses
-- every page of the host into one key: CGI's Njoyn serves every vacancy, and its listing,
-- at /corp/xweb/xweb.asp?...&Jobid=<id>, so the one Njoyn posting the catalogue held
-- (via Welcome to the Jungle) answered /jobs/find for every CGI page — the listing,
-- postings we carry, and postings we do not.
--
-- The key is now host/path?k1=v1&k2=v2: still lowercased, scheme-, www.-, fragment- and
-- trailing-slash-free as before, with the query reduced to its NON-tracking parameters,
-- sorted, and the "?" dropped when none remain. Sorting makes parameter order noise;
-- lowercasing the whole URL (as 0042 did) makes Njoyn's `Jobid`/`jobid` one key.
--
-- What is a tracking parameter is internal/ingest/linksource's trackingParams plus the
-- utm_ prefix — the vocabulary link import already uses for "how a link travelled, not
-- what it points at". An IMMUTABLE index function cannot read it from anywhere, so the
-- list is repeated here, and an integration test in internal/api/handler fails when the
-- two disagree. Adding a key there means a new migration replacing this function.
--
-- The aggregate sorts under COLLATE "C" so the key does not depend on the database's
-- collation (or a glibc upgrade changing it), which an expression index must not.
--
-- Equality on this key is exact: a page carrying a non-tracking parameter its stored url
-- lacks (or the reverse) no longer matches. That miss is the intended trade — the lookup
-- answers exactly or not at all.
CREATE OR REPLACE FUNCTION public.normalize_job_url(text) RETURNS text
    LANGUAGE sql IMMUTABLE PARALLEL SAFE STRICT
    AS $$
        SELECT regexp_replace(regexp_replace(split_part(s.u, '?', 1), '^https?://(www\.)?', ''), '/+$', '')
            || COALESCE('?' || (
                   SELECT string_agg(p, '&' ORDER BY p COLLATE "C")
                   FROM regexp_split_to_table(substring(s.u FROM '\?(.*)$'), '&') AS p
                   WHERE p <> ''
                     AND split_part(p, '=', 1) NOT LIKE 'utm\_%'
                     AND split_part(p, '=', 1) <> ALL (ARRAY[
                         '_hsenc', '_hsmi', 'fbclid', 'gclid', 'gh_src', 'lever-origin',
                         'lever-source', 'li_fat_id', 'mc_cid', 'mc_eid', 'msclkid', 'ref',
                         'refid', 'source', 'src', 'trackingid', 'trk', 'ttclid', 'yclid'
                     ])
               ), '')
        FROM (SELECT lower(regexp_replace($1, '#.*$', '')) AS u) AS s
    $$;

-- Every index over the function holds keys computed by the old body, so each is rebuilt.
-- Until a rebuild finishes, a lookup whose key now carries a query misses the rows still
-- indexed under the old, query-less key — a miss, never a wrong match. A URL with no
-- identifying query keys alike under both bodies.
--
-- CONCURRENTLY, hence the marker: jobs is written by every ingest run. A rebuild killed
-- midway leaves an invalid <index>_ccnew behind; DROP INDEX it and re-run this file, which
-- is idempotent (the function is replaced with the same body, the indexes rebuilt again).
REINDEX INDEX CONCURRENTLY public.jobs_normalized_url_idx;
REINDEX INDEX CONCURRENTLY public.jobs_normalized_url_open_idx;
REINDEX INDEX CONCURRENTLY public.jobs_linkedin_normalized_url_idx;
