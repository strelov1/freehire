## Why

`GET /api/v1/jobs/find?url=` is how the browser extension recognises that the page in
front of the user is a job we already carry, so it can show the curated match card
instead of an ad-hoc text match scraped off the page. It resolves the URL to the
catalog's dedup identity `(source, external_id)` via `sources.RefFromURL`, which
understands exactly one ATS: Greenhouse. Every other host answers `{"data": null}`.

The catalog has 175 source adapters. Measured on prod today:

```
find?url=…himalayas.app/companies/mindera/jobs/staff-java-backend-developer  → null
```

— while that very posting is in the catalog as
`staff-java-backend-developer-mindera-oxtftzij` (source `himalayas`). The extension
therefore falls back to scraping the page and matching its text, which on that page
yields a card titled "This page" at 0%. The posting is ours; we just cannot see it.

Growing `RefFromURL` one provider at a time does not close this: each ATS numbers and
addresses postings differently, so 174 hand-written URL parsers is the price of full
coverage. But the catalog already stores the posting's own detail URL in `jobs.url` —
for most sources that is the very page the user is standing on.

## What Changes

- `FindJob` gains a **second-tier lookup**: when `RefFromURL` cannot name an identity,
  resolve the page URL against `jobs.url` compared in a normalized form (scheme, `www.`,
  query, fragment and trailing slashes removed, lowercased). The exact-identity path stays
  first — it is unaffected by whatever a source chose to store in `url`.
- Normalization is defined **once**, as an `IMMUTABLE` SQL function
  `normalize_job_url(text)`, used by both the index expression and the query — applied to
  the column and to the caller's URL alike, so the two can never drift apart.
- A partial expression index over open, canonical rows
  (`closed_at IS NULL AND duplicate_of IS NULL`) serves the lookup.
- A closed or duplicate posting stays unresolved: the curated card is for a vacancy the
  user can still apply to, and the extension's text-match fallback already covers the rest.

## Capabilities

### New Capabilities
- `posting-url-resolution`: resolving the URL of a job page in the wild to the catalog
  posting it is, by exact source identity first and by the posting's stored detail URL
  second.

### Modified Capabilities
<!-- none — /jobs/find keeps its wire shape ({"data": {"public_slug"}} or {"data": null});
     only the set of URLs it can answer for grows. -->

## Impact

- `migrations/` — one migration: the `normalize_job_url` function plus the partial
  expression index on `jobs`.
- `internal/db/queries/jobs.sql` — new `FindOpenJobByURL` query; regenerate sqlc.
- `internal/handler/find_job.go` — fall through to the URL lookup when no identity is
  recognised.
- No wire change, no change to ingest, search, or dedup.

## Non-Goals

- **No new per-provider URL parser, himalayas included.** Himalayas' `external_id` *is*
  the posting's page URL (its feed `guid`), and `jobs.url` holds the same value, so the
  fallback already resolves it — a hand-written adapter would be a second way to compute
  an answer we now get for free. `RefFromURL` keeps its Greenhouse entry and stays the
  place for URLs whose page address is *not* what `jobs.url` holds (the Greenhouse embed
  form `/embed/job_app?for=…&token=…` is exactly that case).
- **Importing a posting we do not carry.** The extension will offer to contribute an
  unknown page; that is a separate change built on `internal/linksource`.
