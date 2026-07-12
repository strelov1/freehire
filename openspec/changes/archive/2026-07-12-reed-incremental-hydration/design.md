## Context

The `reed` adapter (`internal/sources/reed.go`) is a boardless, keyed aggregator over
the Reed Jobseeker API. It enumerates a curated IT keyword slice, unions the job ids,
and fetches each unique job's **detail** for the full description and employer URL. It
currently implements `StreamingSource` (`FetchStream`), so the pipeline persists postings
incrementally as they resolve.

Two facts collide:
- The Reed API enforces a **per-hour request quota**, signalled with HTTP 403
  (`"exceeded your per-hour request limit"`) — verified live against the prod key.
- The crawl re-fetches detail for all ~1700 live postings every run, and the ingest
  timer runs hourly. This dominant, repeated cost exhausts the quota; when it runs out on
  the first keyword search (before the first emit), the streaming path records
  `"streaming board failed with no progress"` and `board_health` intermittently backs the
  board off.

The pipeline already has the exact seam to fix this. `HydratingSource.FetchNew(ctx, e,
seen)` lets an adapter fetch detail only for postings not in the catalogue; the pipeline
supplies the provider's seen-set (`seenLookup.ExistingExternalIDs`) and, for postings the
adapter re-lists but marks `SeenRefresh`, refreshes liveness by identity via `touch`
(no content rewrite). `justjoin` (~20k live offers) already runs on this path in prod.

## Goals / Non-Goals

**Goals:**
- Bound reed's per-run request volume so it stays under the Reed per-hour quota.
- Eliminate the intermittent "no progress" board_health failures for reed.
- Reuse the existing hydrating seam with zero pipeline changes.

**Non-Goals:**
- Reducing the keyword-search (listing) request count — the search still pages all
  curated keywords each run. That is the irreducible listing cost; only detail is cut.
- A generic per-source rate limiter or 403-retry (a per-hour quota can't be ridden out
  with in-run backoff; wrong tool). Noted as a future lever only.
- Changing usajobs (a healthy sibling with a more generous quota).

## Decisions

**Decision: Convert reed from `StreamingSource` to `HydratingSource`.**
The pipeline dispatch checks `StreamingSource` **first** (`pipeline.go:195`) and returns
before it ever consults `HydratingSource`. So an adapter cannot be effectively both:
reed must drop `FetchStream` for `FetchNew` to run.

- `FetchNew(ctx, e, seen)`: `ids := searchIDs(ctx)` (unchanged union-by-keyword, deduped
  by job id), then `fetchDetails(ids, reedDetailWorkers, fn)` where `fn(id)`:
  - if `seen(strconv(id))` → return a minimal `Job{ExternalID: strconv(id), SeenRefresh:
    true}` (the pipeline refreshes `last_seen_at`/reopen by identity; no detail request,
    no content rewrite);
  - else → `detail(ctx, id)` as today.
- `seen` receives the **raw** numeric id; the pipeline namespaces it via
  `NamespaceExternalID(e.Board, id)` — for boardless reed that is `":<id>"`, matching the
  stored `external_id`.
- `Fetch` (the required `Source` fallback, used when the Store can't supply a seen-set —
  tests / non-DB) reduces to `FetchNew(ctx, e, func(string) bool { return false })`,
  hydrating everything as before.

*Alternatives considered:* (a) a new `StreamingHydratingSource` interface + pipeline
branch — a new abstraction for one adapter, rejected as overkill; (b) making
`FetchStream` seen-aware — changes the `StreamingSource` interface, blast radius across
eightfold/jobtech, rejected as non-surgical.

**Decision: A minimal `SeenRefresh` job carries only `ExternalID`.**
`touch` refreshes by `jobIdentity(e, j)` (source + namespaced external_id) only — it never
reads content — so a SeenRefresh reed job needs no title/company/url. This also honors the
`SeenRefresh` contract: it must NOT carry content that could be re-upserted and wipe the
hydrated description.

## Risks / Trade-offs

- **Loss of streaming incremental-save for reed** → Acceptable: the hydrating crawl only
  fetches detail for new postings, so it is small and rarely reaches the quota mid-run;
  the 48h unseen-sweep already guards a short/interrupted crawl from mass-closing the tail.
- **Listing (search) alone could theoretically still approach the quota** → Mitigated by
  the paired 6h cadence; if it ever surfaces, the noted future lever (trim `reedKeywords`
  or throttle search) applies. Not built now (YAGNI).
- **A first-run against a fresh/empty catalogue hydrates everything** → Same as today's
  behavior and bounded by the 6h cadence; converges after the first run.

## Migration Plan

Pure code change to one adapter; no DB migration, no config. Deploy with the normal
blue/green release. Rollback = revert the adapter change (the pipeline seam is unchanged
and reed simply reverts to streaming). The ops cadence change (hourly→6h) is already
applied and is independent.
