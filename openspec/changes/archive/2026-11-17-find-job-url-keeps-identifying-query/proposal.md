## Why

`/api/v1/jobs/find` matches a page URL against `jobs.url` through `normalize_job_url`
(migration 0042), which drops the WHOLE query string. On a site where the query is what
names the posting — CGI's Njoyn (`xweb.asp?...&Jobid=J1026-1962`), and every classic
`?id=`-style career portal — every page of the host normalizes to the same key, so the
one posting we hold there answers for all of them: the listing, postings we carry and
postings we do not (freehire#4821, reproduced on prod 2026-11-17). The extension then
shows a wrong vacancy on every such page and never offers to add the real one.

## What Changes

- `normalize_job_url` keeps the query string, minus tracking parameters, with the
  remaining parameters sorted, so two links to one posting still match regardless of
  tracking tags or parameter order, while two postings that differ only in the query no
  longer collapse.
- The tracking-parameter vocabulary is the one `internal/ingest/linksource` already uses
  to decide that a parameter "says how a link travelled rather than what it points at";
  the SQL function mirrors it and an integration test holds the two together.
- A no-transaction migration replaces the function and rebuilds both expression indexes
  over it (`jobs_normalized_url_idx`, `jobs_normalized_url_open_idx`) with
  `REINDEX INDEX CONCURRENTLY`.

## Capabilities

### Modified Capabilities

- `posting-url-resolution`: the normalized form keeps non-tracking query parameters.

## Impact

- `migrations/` — one new no-transaction migration.
- `internal/ingest/linksource` — the tracking vocabulary is exported (names only) and
  gains a few well-known tracking keys.
- `internal/platform/db/queries/jobs.sql` — comment only; the query is unchanged.
- A link that previously matched only because a NON-tracking parameter was discarded
  (e.g. the page carries `lang=1` the stored URL lacks) now misses. That is the intended
  trade: the lookup answers exactly or not at all, never a guess.
