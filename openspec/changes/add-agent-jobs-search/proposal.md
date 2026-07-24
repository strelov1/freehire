## Why

`GET /api/v1/jobs/search` returns a truncated `description` — the Meilisearch
document caps the indexed text so the index (and a reindex's transient disk) stays
small; on the production catalogue the full text would add tens of GiB to the
index. That default is correct for the website, which renders only a preview card,
but it leaves programmatic/agent consumers ([#1104](https://github.com/strelov1/freehire/issues/1104))
without the full description from search — their only workaround is a per-result
follow-up fetch by slug.

Rather than bolt a `full_description` flag (and, inevitably, format flags) onto the
SPA-facing search endpoint, this introduces a dedicated agent-oriented search
surface: the same query and filters, but full descriptions and a choice of output
format. The website's search stays lean and Meili-only; the full-fidelity path
lives on its own endpoint.

## What Changes

- New public endpoint `GET /api/v1/agent/jobs/search` (no auth), accepting the
  same free-text query, facet filters, sort, semantic ratio, and `limit`/`offset`
  as the public `/jobs/search`.
- Each result carries the job's **full** `description` (verbatim from Postgres,
  hydrated in one batch query keyed by internal id), never the truncated index
  preview. Hydration is best-effort: a hit whose id has no Postgres row (index lag
  vs a just-removed job) keeps whatever the index served rather than being dropped.
- A `description_format` parameter selects the description representation:
  `html` (default — the stored verbatim HTML), `text` (tags stripped to plain
  text), or `markdown` (HTML converted to Markdown, preserving list/heading
  structure — the friendliest form for LLM consumers).
- Response uses the standard list envelope `{"data": [...], "meta": {...}}` — the
  same wire shape and public-slug identity as the other job reads, not a new
  JSON:API-style envelope.

## Capabilities

### New Capabilities
- `agent-jobs-search`: a dedicated public search endpoint for programmatic/agent
  consumers that returns full job descriptions with a selectable output format.

### Modified Capabilities
<!-- none — the existing job-search endpoint and its truncated preview are unchanged -->

## Impact

- **API**: adds `GET /api/v1/agent/jobs/search`. Additive; no change to
  `/jobs/search` or any existing caller.
- **Code**: `internal/handler` (new handler + route, reusing the existing
  `buildSearchFilter`/`searchSort`/`pageParams` and the search backend), a batch
  description loader in `internal/db` (narrow `id, description` query), and an
  HTML→text/markdown conversion helper.
- **Dependencies**: a small HTML-to-text/Markdown conversion library (for the
  `text`/`markdown` formats).
- **Data stores**: the endpoint reads Postgres for the returned page's
  descriptions (bounded by the page limit) in addition to the Meili query; the
  public `/jobs/search` remains Meili-only.
- **Docs**: the `api-documentation` surface gains the new endpoint and its
  `description_format` parameter.
