-- Bound saved_searches.query the same way name has been bounded since 0001
-- (saved_searches_name_check): the query column had no length limit anywhere, Go or
-- SQL, even though it is client-supplied and internal/notify's matching pass re-parses
-- every distinct query (url.ParseQuery) on every scheduled run. internal/savedsearch
-- now rejects a query over maxQueryLen (2000) before it reaches SQL; this CHECK is the
-- backstop for any other writer.
--
-- NOT VALID for the same reason 0056 used it for user_profiles' skill-cardinality
-- backstop: it enforces the bound on every INSERT/UPDATE from here on without a
-- validation scan or a lengthy lock, and cannot fail the migration on a legacy row
-- written before the application-side cap existed. To promote it later, once prod is
-- known clean: ALTER TABLE public.saved_searches VALIDATE CONSTRAINT saved_searches_query_check.
ALTER TABLE public.saved_searches
    ADD CONSTRAINT saved_searches_query_check
    CHECK (length(query) <= 2000) NOT VALID;
