## Why

The jobs browse list orders by `created_at` (when a posting entered our
catalogue), so the order reflects crawl timing rather than how fresh a job
actually is — a posting published yesterday can sit below one published months
ago that we happened to ingest later. Rather than pick a clever implicit
ordering (e.g. a `COALESCE(posted_at, created_at)` fallback), we give the user an
explicit sort control so the result is simple and predictable: they choose the
field, they get exactly that order.

## What Changes

- Add a **sort control** to the jobs browse UI (`JobsView`) offering two
  options: **Date posted** (`posted_at`) and **Recently added** (`created_at`),
  both newest-first. The choice is mirrored into the URL query (like the other
  filters) so it survives reload, sharing, and back/forward.
- Change the search endpoint's **empty-query default** ordering from
  `created_at:desc` to `posted_at:desc`, so the default browse surfaces the
  freshest postings first. An explicit `sort` still takes precedence; a text
  query still keeps relevance order.
- Decouple the **DB-backed list** (`/api/v1/jobs`) ordering from the search
  default: it keeps `created_at:desc` as the API's stable, keyset-friendly
  default (the SPA browses via search, not this endpoint).

Not breaking: the search API already accepts both `posted_at` and `created_at`
as `sort` values; only the no-sort default shifts.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities

- `job-search`: the no-sort, empty-query default ordering changes to
  `posted_at` descending; the DB-list ordering is no longer required to match
  the search default.
- `web-frontend`: the jobs browse UI gains a URL-synced sort control over
  `posted_at` / `created_at`.

## Impact

- `internal/handler/search.go` (`searchSort`): empty-query default →
  `posted_at:desc`; its test (`search_test.go`) updates accordingly.
- `web/src/lib/filters.svelte.ts`: `sort` added to the filter model, its
  URL (de)serialization, and a setter.
- `web/src/lib/components/JobsView.svelte`: the sort `<select>` control.
- No DB migration, no Meilisearch reindex (both fields already sortable).
