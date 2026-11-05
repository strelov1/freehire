-- cmd/reindex's time-crossings pass keeps its cursor here too: the row named
-- 'search-time-crossings' holds an RFC 3339 instant in after_company, the end of the window
-- its last run covered, rather than a company slug.
COMMENT ON TABLE marker_pass_cursors IS
    'Resume point of a cmd/reindex pass. Per-company duplicate-marker passes store the last '
    'company a run finished (empty means start from the top); the search-time-crossings pass '
    'stores the RFC 3339 instant its last run covered up to.';
