-- migrate: no-transaction
-- The application URL is also a posting identity for extension navigation. Build
-- concurrently: the catalogue is large, but only known destinations enter this index.
-- Keep unknown query parameters (including vacancy IDs) and path case. Only known
-- utm_* and LinkedIn trk parameters are tracking; repeated identity keys retain order.
CREATE OR REPLACE FUNCTION public.normalize_application_url(text) RETURNS text
    LANGUAGE sql IMMUTABLE PARALLEL SAFE STRICT
    AS $$
        WITH parts AS (
            SELECT regexp_replace(split_part(split_part($1, '#', 1), '?', 1),
                                  '^https?://', '', 'i') AS authority_path,
                   COALESCE(substring(split_part($1, '#', 1) FROM '[?](.*)$'), '') AS query
        ), canonical AS (
            SELECT regexp_replace(lower(split_part(authority_path, '/', 1)), '^www[.]', '')
                       || regexp_replace(COALESCE(substring(authority_path FROM '/.*$'), ''), '/+$', '') AS base,
                   (SELECT string_agg(param, '&' ORDER BY split_part(param, '=', 1) COLLATE "C", position)
                    FROM unnest(string_to_array(query, '&')) WITH ORDINALITY AS p(param, position)
                    WHERE param <> '' AND lower(split_part(param, '=', 1)) !~ '^(utm_|trk$)') AS query
            FROM parts
        )
        SELECT base || CASE WHEN query IS NULL THEN '' ELSE '?' || query END FROM canonical
    $$;

CREATE INDEX CONCURRENTLY IF NOT EXISTS jobs_normalized_application_url_idx
    ON public.jobs (public.normalize_application_url(application_url))
    WHERE application_url <> '';
