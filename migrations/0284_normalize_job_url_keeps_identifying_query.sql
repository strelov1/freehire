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
-- An HTML-escaped separator (`&amp;`, as a url scraped out of markup can carry) is read as
-- the `&` it stands for, so it does not turn the next parameter into an `amp;<key>`.
--
-- The aggregate sorts under COLLATE "C" so the key does not depend on the database's
-- collation (or a glibc upgrade changing it), which an expression index must not.
--
-- The key is bounded. A B-tree entry must fit about a third of a page (~2.7 kB), and a
-- kept query can be long (a signed token, a serialized search state); one oversized key
-- would fail this file's REINDEX and then every write of that posting. A key over 2000
-- bytes becomes its first 500 characters (at most 2000 bytes even in 4-byte UTF-8) plus
-- the md5 of the whole key: still exact for equality, since both sides go through it.
--
-- Equality on this key is exact: a page carrying a non-tracking parameter its stored url
-- lacks (or the reverse) no longer matches. That miss is the intended trade — the lookup
-- answers exactly or not at all.
CREATE OR REPLACE FUNCTION public.normalize_job_url(text) RETURNS text
    LANGUAGE sql IMMUTABLE PARALLEL SAFE STRICT
    AS $$
        SELECT CASE WHEN octet_length(k.key) <= 2000 THEN k.key
                    ELSE left(k.key, 500) || '#' || md5(k.key) END
        FROM (
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
               AS key
        FROM (SELECT replace(lower(regexp_replace($1, '#.*$', '')), '&amp;', '&') AS u) AS s
        ) AS k
    $$;

-- Every index over the function holds keys computed by the old body, so each is rebuilt.
-- An index scan does not recompute the expression, so until a rebuild finishes, rows still
-- indexed under their old, query-less key behave as before this file: a query-bearing
-- lookup misses them, and a lookup whose new key happens to equal an old one (the bare
-- `xweb.asp` against a stored `xweb.asp?jobid=...`) still matches them — the #4821 bug,
-- lingering until the rebuild replaces the index. A rebuild that fails leaves the old index
-- serving; re-running the file is the recovery. A URL with no identifying query keys alike
-- under both bodies.
--
-- CONCURRENTLY, hence the marker: jobs is written by every ingest run. The file is
-- idempotent (the function is replaced with the same body, the indexes rebuilt again).
-- A rebuild killed midway (the runner's lock_timeout aborting its wait on a long ingest
-- transaction, a dropped session) leaves an invalid <index>_ccnew behind, which every write
-- still maintains. Each rebuild therefore first drops its own carcass, so re-running this
-- file is the whole recovery; the runner's invalid-index check cannot see a _ccnew name.
DROP INDEX CONCURRENTLY IF EXISTS public.jobs_normalized_url_idx_ccnew;
REINDEX INDEX CONCURRENTLY public.jobs_normalized_url_idx;
DROP INDEX CONCURRENTLY IF EXISTS public.jobs_normalized_url_open_idx_ccnew;
REINDEX INDEX CONCURRENTLY public.jobs_normalized_url_open_idx;
DROP INDEX CONCURRENTLY IF EXISTS public.jobs_linkedin_normalized_url_idx_ccnew;
REINDEX INDEX CONCURRENTLY public.jobs_linkedin_normalized_url_idx;
