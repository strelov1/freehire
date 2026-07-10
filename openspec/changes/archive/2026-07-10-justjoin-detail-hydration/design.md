## Context

`internal/sources/justjoin.go` reads only the `by-cursor` list endpoint, which omits the
posting body, so every justjoin job is stored with an empty `Description`. The body lives
only in `GET https://api.justjoin.it/v1/offers/{slug}` (verified live — the list `/v2/...`
base 404s for a slug; the detail is a separate `/v1` route). `description` is part of
`jobhash` (the `content_hash` that drives change-detection and search re-indexing) and it
feeds skilltag/enrichment, so it must be present at upsert time, not backfilled into the
JSONB later.

The catalogue holds ~20,515 live justjoin offers. The list already carries the structured
fields (`requiredSkills`, `experienceLevel`, `employmentTypes`, `categoryId`) — only `body`
is detail-only. Fetching detail for every offer every crawl is ~20k extra requests per run
(risking rate-limiting), so the design fetches detail only for offers the catalogue does not
already have.

The pipeline (`internal/pipeline/pipeline.go`) dispatches each board to its adapter via
`Source.Fetch`, then saves each returned job through `Store.Save`. It already expresses
optional adapter/store capabilities as interfaces the runner type-asserts (`StreamingSource`,
`closer`, `BoardHealth`) — the idiom this change follows.

## Goals / Non-Goals

**Goals:**
- justjoin jobs carry a real (sanitized) description.
- Detail requests happen only for new offers in steady state; bounded concurrency; a single
  offer's failure does not abort the crawl.
- Fill the structured facets justjoin states (skills/seniority/category) mapped to freehire's
  vocabularies, since detail is fetched anyway.
- Backfill the ~20k existing empty rows once.
- No change to `Source.Fetch` or to any other adapter; no N+1 DB queries.

**Non-Goals:**
- Fetching detail for already-seen offers on every crawl (rejected: cost).
- Salary parsing from `employmentTypes` (out of scope; separate concern).
- A general framework for detail hydration across all aggregators — only justjoin opts in
  now; the seam is reusable but not retrofitted elsewhere.

## Decisions

**1. Optional `HydratingSource` interface, not a param on `Fetch`.**
Add `HydratingSource { Source; FetchNew(ctx, e, seen func(externalID string) bool) ([]Job, error) }`.
The runner type-asserts it (exactly like `StreamingSource`); justjoin implements both `Fetch`
(list-only, kept for back-compat/tests/fallback) and `FetchNew`. Alternative — add a
`seen`/`knownIDs` parameter to `Source.Fetch` — rejected: it forces a stub arg on every
adapter and muddies the single-purpose fetch signature.

**2. Provider-scoped seen-set, one query per crawl.**
Add an optional Store capability `seenLookup { ExistingExternalIDs(ctx, source) (map[string]struct{}, error) }`,
implemented in `cmd/ingest/store.go` over a new sqlc query `SELECT external_id FROM jobs WHERE source = $1`.
The runner, when both the adapter is a `HydratingSource` and the Store is a `seenLookup`,
loads the set once and closes a `seen` predicate over it (O(1) membership per offer).
Fails open: a lookup error → empty set (every offer treated as new), logged, board still
crawled. Alternative — per-offer `exists` query — rejected as N+1.
Note the seen-set uses the namespaced `external_id` as stored; justjoin is boardless so the
namespace is stable, and the adapter keys on the raw `guid` — the runner adapts between them
(the predicate it passes already accounts for how identity is namespaced).

**3. Detail fetch: bounded concurrency inside `FetchNew`.**
`FetchNew` pages the list as today; for each unseen offer it fetches `/v1/offers/{slug}` via
the shared `JSONGetter`, with a bounded worker pool (small fixed fan-out, in the spirit of
the enrich concurrency), and maps `body`→`Description` (sanitized), `requiredSkills[].name`→
`Skills` (via `skilltag.Parse`), and `experienceLevel.value`→`Seniority` (justjoin's `mid`
means `middle`, then vocab-membership per `enrich.SeniorityValues`; empty when unmapped).
**Category is intentionally not derived from justjoin's `category`**: it is a language/stack
tag (JavaScript/Java/Python) that does not pin a single freehire role category, so mapping it
would guess (JS is frontend or backend). The title dictionary decides category — the current
behavior. A detail error for one offer is logged and that offer
falls back to list-only (still ingested, just without a body this run — it will retry next
crawl because it stays unseen only if never saved; once saved list-only it is seen, so the
backfill/next-change path covers it — acceptable, logged).

**3a. Seen offers refresh liveness only — they must NOT re-upsert content (review correction).**
A seen offer is re-listed every crawl but carries no fresh content (detail is skipped). It cannot
simply be upserted list-only: `jobderive` re-derives the deterministic facets (skills,
posting_language, …) from the empty description, and `UpsertJob` guards only `description`
(`COALESCE(NULLIF(...))`) — every other facet column is written unconditionally, so a content-less
re-upsert would WIPE the description-derived facets hydrated when the offer was new, and churn
`content_hash`. But the offer still needs its `last_seen_at` refreshed, or the 48h unseen sweep
would wrongly close a still-live offer once its company gets any new offer (which puts the company
in the crawled-set). So a seen offer is marked (`sources.Job.SeenRefresh`) and the pipeline routes
it to a liveness-only `Touch` (bump `last_seen_at`, reopen if closed) by identity, leaving content
untouched — mirroring the `Removed → Close` routing. `Touch` is an optional Store capability
(`toucher`), like `closer`. `TouchJob` `RETURNING company_slug` so `dbStore.Touch` records the
company into the crawled-set that scopes the post-run unseen sweep — exactly as `Save` does —
otherwise a company whose offers were all touched (none newly saved this crawl) would fall out of
the sweep and its genuinely-removed offers would never close.

**4. One-time `cmd/backfill-justjoin`.**
Run-once worker: select `source='justjoin'` rows, derive slug from the stored URL, fetch
detail, update description via a new query, isolate/count per-row failures. Followed by
`make reindex`. Separate command (not an `--backfill` flag on `cmd/ingest`) to keep the cron
ingest path unbranched and match the other run-once maintenance commands.

## Risks / Trade-offs

- **First post-deploy state: 20k rows still empty until backfill runs** → run
  `cmd/backfill-justjoin` + `make reindex` as the deploy step (documented in tasks); until
  then only new offers get descriptions.
- **A new offer whose detail fetch fails is saved list-only (empty body) and then counts as
  "seen"** → it will not auto-hydrate next crawl. Mitigation: the failure is logged; the
  periodic backfill (or a future re-hydrate on content change) recovers it. Acceptable given
  detail failures are rare and the alternative (not saving the offer) hides the job entirely.
- **justjoin rate-limiting on the backfill's 20k detail requests** → bounded concurrency +
  isolate-and-continue; the backfill can be re-run (idempotent update) if throttled.
- **`description` moving `content_hash` re-indexes ~20k docs once** → expected and desired;
  the incremental index + reindex reconcile it.

## Migration Plan

1. Ship the adapter + pipeline seam (new offers hydrate going forward).
2. Run `cmd/backfill-justjoin` once against prod, then `make reindex`.
3. Rollback: the change is additive (new interface/queries/command); reverting the binary
   restores list-only ingest with no schema rollback needed (no migration is added).

## Open Questions

None — seam shape (optional port + seen-set) and backfill (separate command) confirmed with
the requester.
