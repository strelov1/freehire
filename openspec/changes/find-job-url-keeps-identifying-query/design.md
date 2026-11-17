## Context

The second tier of `/jobs/find` (and `/jobs/resolve`) is an index lookup:
`normalize_job_url(j.url) = normalize_job_url($1)`, backed by two partial expression
indexes. Equality on one normalized key is the constraint — the lookup cannot do
`internal/ingest/linksource.sameURL`'s asymmetric "the link carries every non-tracking
parameter the stored URL does" without giving up the index.

## Decisions

**Keep the query, drop only tracking parameters, sort the rest.** The key becomes
`host/path?k1=v1&k2=v2` with parameters sorted and tracking ones removed (an empty
remainder drops the `?`). Equality on that key is exact: a page carrying an extra
non-tracking parameter misses. That is preferred over the current false positive, in line
with the tier's "neither tier guesses" rule.

**Lowercase the whole URL, as before.** Keys and values are lowercased with the rest; Njoyn
alone spells `Jobid`/`jobid` and `Page`/`page` both ways.

**One vocabulary, two languages.** `linksource.trackingParams` (+ the `utm_` prefix) is
the existing Go statement of what a tracking parameter is. The SQL function repeats the
list (an IMMUTABLE index function cannot read a table or a setting), and an integration
test in `internal/api/handler` — the one layer that may import `linksource` and has a
Postgres — asserts that SQL strips every name Go lists and keeps identity parameters.
New well-known keys (`lever-source`, `refid`, `trackingid`, `mc_cid`, `_hsenc`, …) are
added to the Go list, which only widens what may not veto a link match there.

**Replace in place + `REINDEX CONCURRENTLY`, not a `_v2` function.** Index names and the
query stay as they are and there is no follow-up migration to retire a v1. During the
reindex, rows indexed under the old key make a query-bearing lookup miss (never match
wrongly: an old key has no query, a new requested key has one); a URL with no
identifying query keys the same under both versions. The file is idempotent — a re-run
replaces the same body and rebuilds again. A killed `REINDEX CONCURRENTLY` leaves a
`*_ccnew` carcass that must be dropped by hand; the statement fails loudly, so the version
is not recorded.

**SQL function with a scalar subquery.** The body aggregates the split query with
`string_agg(... ORDER BY ...)`; a body with a sublink is not inlined, so the index
expression and the query both remain the function call and the planner keeps matching
them. Verified by EXPLAIN in the integration test environment.

## Risks

- Stored URLs carrying a non-tracking parameter the visited page lacks (or the reverse)
  stop matching. Mitigated by the identity tier, which answers most ATS URLs before this
  tier runs, and by the widened tracking vocabulary.
