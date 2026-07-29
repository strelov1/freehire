# Link source conventions

## Scope
Resolving a single outbound job-detail URL into a fully parsed vacancy under the destination's own identity.

## Always true
- `internal/linksource` turns one outbound job-detail URL into a fully parsed vacancy.
- `sources` adapts a whole ATS board by id; a `LinkSource` adapts a single detail page — it matches the link's host and resolves that one page.
- Adding a host-scoped destination is a new adapter plus one line in `linksource.All` — the same shape as `sources.All`.
- The resolved job is stored under the destination's identity, not Telegram's, so it dedups against the same posting if another source also has it.
- **Board coverage is one adapter over every recognised ATS** (`boardcoverage.go`), not fifty
  hand-written ones: derive `(source, board)` via `internal/atsboard`, fetch that tenant's
  board through the ingest adapter that already crawls the platform, and pick the posting the
  link points at. all recognised providers with an ingest adapter, so coverage grows
  with the recogniser table rather than with code.
- **It is the only `PerLinkSource`.** One adapter serves many platforms, so `Source()` cannot
  name the identity a job is stored under — `SourceFor(u)` does, and `ResolveLinks` prefers it.
  Without that, every board-covered import would land under one bogus source.
- **Order is stated once, in `ImportRegistry`**, and pinned by a test: host-scoped adapters
  first (a platform with a cheap per-job API must keep using it), board coverage second (it
  fetches a whole board), `generic` last — nothing may follow it, since it matches every page
  and `Find` commits to a single adapter.
- **A fetched-but-absent posting is `ok=false`; an unfetchable board is an error.** Conflating
  them makes the caller file a reachable vacancy as unimportable.
- **`generic` winning is not the same as nothing matching.** Its `Match` is always true, so a
  page it can read never reaches anything downstream — `linkimport` therefore checks for
  `GenericSource` explicitly, not just for an empty result, before preferring a board the
  caller already resolved. A `len(resolved) == 0` guard alone leaves that branch dead for every
  page carrying a `JobPosting` block, which is most storefronts.
- **A posting id is matched anywhere in the path, last segment first** (`pickPosting`). A
  storefront appends a readable slug after the id (`…/jobs/<id>/<title>/`), so tail-only
  matching reports a posting as absent from its own board. Precision is safe here because the
  comparison is against one board's own ids.

## How it works
A Telegram post often just links to a real vacancy elsewhere. Rather than treating the Telegram post itself as the job, `internal/linksource` follows the outbound URL and resolves the actual detail page at the destination ATS. This reuses the same adapter pattern as `internal/sources` but at the granularity of a single page: a `LinkSource` matches the link's host and parses that one detail page into a normalized job. The job is then stored under the destination source's identity (e.g. greenhouse, lever), not under telegram, so the dedup key `(source, external_id)` naturally prevents duplication if another source also carries the same posting.

## Limitations
- Board coverage answers about one vacancy by fetching a whole tenant board. Acceptable behind
  the per-user hourly budget and the dedicated adapters, but if it proves too costly the seam
  is an optional `FetchOne(board, id)` on `sources.Source`, type-asserted the way
  `StreamingSource`/`HydratingSource` already are.
- A vanity careers domain cannot be handled by an adapter at all: `Match` is a pure offline
  predicate and `ResolveLinks` commits to the one adapter `Find` returns, so the
  `boardresolve` page fetch is orchestrated a level up, in `internal/linkimport`.
