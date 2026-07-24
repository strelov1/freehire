## Context

`GET /api/v1/jobs/search` (`internal/handler/search.go`) runs a Meilisearch query
and maps each hit's `jobview.Job` into the list envelope. The indexed document
caps `description` at `maxIndexedDescriptionRunes` (`internal/search/document.go`),
so search hits carry a truncated preview; the full text lives in Postgres and is
served by the DB-backed list and detail endpoints. Each `search.JobDocument` hit
carries the internal `ID` (the Meili primary key = `jobs.id`), so the returned ids
are available in rank order for a follow-up read.

The public search endpoint's request handling (filter build, sort, pagination
window, semantic ratio) is the contract this new endpoint must match exactly.

## Goals / Non-Goals

**Goals:**
- A public search endpoint that returns full descriptions with a selectable
  format (`html`/`text`/`markdown`), running the identical query/filter/sort/paging
  as the public search.
- Keep the public `/jobs/search` unchanged: still Meili-only, still preview-only.
- Structure the shared search logic so the two endpoints cannot drift.

**Non-Goals:**
- No auth/API key (public, keyless) — full descriptions are already public via the
  detail endpoint.
- No field selection, cursor pagination, or JSON:API envelope — same `{data, meta}`
  shape; those remain a future seam if a concrete need appears.
- No change to the index, the truncation cap, or the public search endpoint.

## Decisions

**Extract the shared search core; both endpoints call it.** Factor the public
`SearchJobs` body (parse params → `buildSearchFilter`/`searchSort`/`pageParams`/
window guard → `a.search.Search` → `SearchResult`) into one internal helper that
returns the hits and total. `SearchJobs` maps hits to views as today; the new
`AgentSearchJobs` calls the same helper, then hydrates + formats. This makes "runs
the same search" a structural guarantee, not a copy that can drift. Chosen over
duplicating the request handling (the spec requires parity, and drift is the
obvious failure mode).

**Hydrate full descriptions from Postgres by id.** Reuse the batch pattern: collect
the hits' ids, run a narrow `GetJobDescriptionsByIDs(ids) -> (id, description)`
query, and replace each hit's `Description` with the full text. Best-effort by id
(a missing row keeps the index value, never drops the hit). Narrow projection
avoids detoasting the wide row and re-deriving the view. Bounded by the page limit,
so it is at most a page of primary-key lookups — an indexed batch.

**Format conversion in a small dedicated helper.** `description_format` maps to a
pure `formatDescription(html, format) string`:
- `html` → identity (verbatim stored HTML).
- `text` → strip tags to plain text.
- `markdown` → convert HTML to Markdown, preserving block structure.
Isolating this behind one function keeps the handler thin and the conversion
independently testable. An unrecognized value falls back to `html` (lenient, never
errors — consistent with how the search endpoint ignores unknown sort params).
The HTML→Markdown/text conversion uses a small library; the choice is an
implementation detail of this helper and does not leak into the handler.

**Route placement.** Register `GET /api/v1/agent/jobs/search` alongside the public
search route, public (no auth middleware). The `/agent` path segment names the
programmatic surface without implying authentication.

## Risks / Trade-offs

- [The agent endpoint reads Postgres on every request] → It is a separate endpoint;
  the public `/jobs/search` stays Meili-only. The read is bounded by the page limit
  and is by primary key. Acceptable for a full-fidelity programmatic surface.
- [Refactoring `SearchJobs` to share a core could regress the public endpoint] →
  The existing `search_test.go` suite pins the public endpoint's behavior; the
  refactor must keep it green. Extract-and-reuse, do not rewrite request handling.
- [Format conversion cost per response] → Bounded by page size (≤ the search page
  limit); `markdown`/`text` convert only the returned page. `html` (default) is a
  no-op. Negligible relative to the Meili + Postgres round-trips already on the path.
- [A new dependency for HTML conversion] → Small, well-scoped library used only in
  the format helper. If undesirable, `text` can fall back to a standard-library
  tag strip; `markdown` is the only format that genuinely needs a converter.

## Migration Plan

Additive: a new endpoint and a new capability, no change to existing endpoints,
schema, or the index. No data migration. Rollback is removing the route, handler,
query, and helper; nothing persisted depends on it.

## Open Questions

None.
