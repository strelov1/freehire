## Context

The `/companies` page (`web/src/lib/components/CompaniesView.svelte`) renders a
paginated list backed by `GET /api/v1/companies`. The jobs list already has a
search box wired to a separate `GET /api/v1/jobs/search` endpoint backed by
Meilisearch, with filter state mirrored to the URL via `FilterStore` +
`router.setQuery`. Companies have only `slug`, `name`, and a computed
`job_count` — there is no Meilisearch index for them.

## Goals / Non-Goals

**Goals:**

- Find a company by typing part of its name on `/companies`.
- Keep the search shareable/bookmarkable via the URL (`?q=`) and correct under
  back/forward.
- Mirror the jobs page's interaction pattern so the UI feels consistent.

**Non-Goals:**

- No Meilisearch index for companies.
- No fuzzy/typo-tolerant ranking — a substring name match is enough.
- No new filters beyond name (no facets, no sort options).

## Decisions

### Extend the existing list endpoint instead of a new `/companies/search`

`GET /api/v1/companies` gains an optional `?q=`; empty/absent `q` is unchanged.
Jobs has a separate `/jobs/search` because Meilisearch is a fundamentally
different backend from the SQL list. Companies search is the *same* SQL source
with a `WHERE` clause, so a second route, handler, and response shape would be
duplication. One endpoint also keeps `meta.total`/pagination uniform.

*Alternative considered:* a dedicated `/companies/search` mirroring jobs.
Rejected — no separate backend to justify the split.

### SQL `ILIKE` with an optional-filter predicate

Both `ListCompanies` and `CountCompanies` get
`WHERE sqlc.arg('search')::text = '' OR c.name ILIKE '%' || sqlc.arg('search') || '%'`.
This is the idiomatic sqlc way to make a filter optional without dynamic SQL:
one prepared statement serves both "list" and "search", and Postgres
short-circuits the `ILIKE` when `search` is empty. Switching `ListCompanies` to
named sqlc args (`search`/`limit`/`offset`) keeps the regenerated params struct
readable now that the signature changes.

*Alternative considered:* full-text (`to_tsvector`) or trigram (`pg_trgm`).
Rejected — overkill for a short name field; `ILIKE` needs no extension or index
change at this scale.

### Frontend: inline URL sync, not `FilterStore`

`CompaniesView` mirrors `JobsView`'s structure but inlines a single `q` state
rather than reaching for `FilterStore` (which models facets, exclude modes, and
salary — none of which apply). Two `$effect`s form a controlled URL↔state loop:
one pulls `q` from `router.search` on back/forward (guarded by `if (urlQ !== q)`
to break the write-back loop), the other writes `?q=` via `router.setQuery` and,
debounced 300ms, recreates the `Paginator` with the new `q`. This is the same
guard `FilterStore.syncFromUrl` uses (param-string comparison), scaled down to
one field.

*Alternative considered:* reuse `FilterStore`. Rejected — it would couple the
companies page to job-specific filter concepts (facets, FACETS list).

### `api.ts`: `listCompanies(q, limit, offset)`

`listCompanies` gains a leading `q` argument and builds `?q=&limit=&offset=`,
omitting `q` when empty. The `Paginator` closure passes the current `q`.

## Risks / Trade-offs

- **User-entered `%`/`_` act as LIKE wildcards** → Accepted. The query is
  parameterized (no injection); wildcard chars in a company-name search are
  harmless and rare. Escaping is a noted seam, not built now.
- **`meta.total` semantics shift** (was always all companies; now the filtered
  count) → Intended — pagination over a filtered set requires the filtered
  total. The empty-query path returns the same total as before.
- **Two-`$effect` URL loop could double-fire** → Mitigated by the
  `if (urlQ !== q)` guard, mirroring the proven `FilterStore.syncFromUrl`
  pattern.
