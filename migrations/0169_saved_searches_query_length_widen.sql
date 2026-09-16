-- Widen saved_searches.query's bound from 2000 to 4000 (internal/search/savedsearch's
-- maxQueryLen). The original 0096 bound was tripped by a real profile-derived query: a
-- rich profile can hold up to 200 skills and, independently, up to 200 excluded skills
-- (userprofile.go's own caps, free text with no dictionary check), and "notify me about
-- jobs matching my profile" (ProfileAlertToggle) folds both straight into the saved
-- search it creates. The frontend (facetModel.ts's skillCharBudget) now separately
-- bounds how much of a profile's own skills it ever feeds into that query — 4000 is
-- headroom for a legitimately large filter, not a substitute for that cap, and stays far
-- short of the point where re-parsing the query on every internal/engage/notify pass
-- (url.ParseQuery, once per distinct query per pass) becomes costly.
--
-- The 0096 constraint was itself never VALIDATEd (see its own comment), so there is no
-- validation scan to redo here either: dropping and re-adding NOT VALID takes neither a
-- lock nor a scan, and cannot fail on a legacy row.
ALTER TABLE public.saved_searches
    DROP CONSTRAINT saved_searches_query_check;

ALTER TABLE public.saved_searches
    ADD CONSTRAINT saved_searches_query_check
    CHECK (length(query) <= 4000) NOT VALID;
