## Context

Two listing surfaces already attach the ghost signal to their cards:

- `jobsHandlers.attachGhostToRows` (`internal/api/handler/jobs.go`) — for `GET /jobs`, which reads full `db.Job` rows from Postgres (so it can call `jobview.ClassifyReality` directly off the row's own description).
- `searchHandlers.attachGhost` (`internal/api/handler/search.go`) — for the filtered search, which reads `search.JobDocument` hits from Meilisearch. Its own comment: "The reality class... IS on the document (`search.FromJob` promotes it), so it is read from the hit rather than recomputed."

Both share `ghostEvidenceFor` (`internal/api/handler/ghost_evidence.go`) for the outcome-evidence half of the signal, and both read `ListJobGhostStamps` for the closed/`ats_absent_at` stamps — that shared function already documents why it exists ("Two copies of this drifted apart the moment one of them gained the closed-job check").

The tracker (`ListTrackedJobs`, `internal/api/handler/me_tracking.go`) has neither a full `db.Job` row (its query deliberately reads only the card's columns — no description) nor live Meilisearch hits (its rows come from `user_jobs`/`applications`, not a search). It knows the tracked jobs' **ids**, though — `jobtracking.TrackedJob` embeds `Interaction{ JobID int64, ... }`.

`id` is already a filterable Meilisearch attribute (`internal/search/search/client.go`'s `FilterableAttributes`, whose own comment says it already backs "the swipe deck's `id NOT IN [...]` per-user exclusion" — `internal/api/handler/swipe.go` + `search.NotIn`, `internal/search/search/filter.go`). There is no `In` counterpart to `NotIn` yet.

`trackingHandlers` (`internal/api/handler/user_jobs.go`) already holds a `search searcher` field (used today only by `SwipeDeck`) but does not store `*db.Queries` — `newTrackingHandlers` takes it as a parameter and only threads it into `jobtracking.New`/`reminder.New`.

## Goals / Non-Goals

**Goals:**
- A tracked job's card carries the same `ghost` signal the job's own detail page and the other two listings already show, computed the same way (`jobview.ClassifyGhost`), never a bespoke derivation.
- No new read of a job's description anywhere in this path — `TestMeasureBoardLoad` stays green with no changes.
- Best-effort: a lookup failure (search unavailable, a query error) degrades to omitting `ghost` from the affected cards, exactly like `ghostEvidenceFor`'s own established discipline — never a failed listing request.
- Bounded cost: one Meilisearch query and the two existing ghost-evidence queries per page, not per row.

**Non-Goals:**
- Serving `Reality` on the tracker card. Neither `/jobs` nor search does this today (Context, above) — both use `ClassifyReality`'s output only to feed `ClassifyGhost`'s `RealityClass` input. This change follows that convention rather than introducing a new one.
- Changing `ghost.Classify`, `jobview.ClassifyGhost`, or the ghost-signal's own derivation rules. Reused verbatim.
- A live "is this job still in the index" check. A tracked job absent from Meilisearch (not yet reindexed, or the index and Postgres have briefly diverged) simply reads `RealityClass = ""` for that one job, the same degrade an ordinary missing-hit already produces elsewhere.

## Decisions

**Add `search.In(attr string, ids []int64) string`, mirroring `NotIn` exactly (same file, same shape, same escaping-free numeric-id path).** Not a generalized `In`/`NotIn` unification — the two already read as mirror-image siblings, and forcing them through one parameterized helper for two call sites each is not a simplification this change needs to make.

**Fetch reality classes with one `Search` call filtered to `id IN [tracked ids]`, `Limit: len(ids)`, no query text, no sort.** This is not a "search" in the query sense — it is the existing `searcher` interface used as a batch document-by-id read, the same way `sitemap.go` already uses Meilisearch's own `GetDocuments` for a different bulk-by-id need. Reusing `Search` (already the one method `searcher` exposes to this handler) over adding a second capability to the interface for one caller.

**Give `trackingHandlers` a `queries *db.Queries` field**, populated in `newTrackingHandlers` from the parameter it already receives. Matches `jobsHandlers`/`searchHandlers`, which already hold one for the identical purpose (`ghostEvidenceFor`, `ListJobGhostStamps`).

**The attach pass lives in `me_tracking.go`, as a new unexported method on `trackingHandlers`, called from `ListTrackedJobs` after `items` is built.** Not inside `jobtracking.Service`/`QueriesRepository`: `jobtracking` is `application`-block, `internal/search` is also `application`-block (sibling, same layer) — the layering rule forbids either from importing the other. `internal/api/handler` sits above both (layer 8) and already holds exactly this shape of cross-cutting attach step for the other two listings, so the tracker's belongs there too, not inside the tracking service.

**Skip cards with no `Job`** (an orphaned application whose posting `cmd/prune` removed) **before collecting ids.** There is nothing to attach a signal to — `TrackedJob.Job` is nil for exactly these rows already.

**Degrade order, matching `ghostEvidenceFor`'s own convention:** `h.search == nil` → skip the whole attach pass (mirrors `SwipeDeck`'s existing `h.search == nil` check, and `ghostEvidenceFor`'s own `q == nil` check). A `Search` error → log via the same `logGhostLookup` helper, leave every card's `RealityClass` at `""` (ghost may still fire off ATS-absence/evidence alone) rather than aborting the request. `ListJobGhostStamps`/`ghostEvidenceFor` failures already degrade this way today; nothing new to add there beyond calling them.

## Risks / Trade-offs

- **[Risk]** A tracked job whose Meilisearch document is momentarily stale or absent under-reports `evergreen_posting` for that one card. → **Mitigation**: not a new risk — the search-backed listing already carries the identical exposure for every card it serves, and `ghost.Classify` never fires that criterion off missing data (it fires off `RealityClass == "likely-evergreen"`, so absence is silence, not a false badge).
- **[Trade-off]** One extra Meilisearch round trip per tracking-page load, on top of the two ghost-evidence queries the search listing already accepts. → Accepted: bounded to the page size (≤500, `trackingMaxLimit`), the same shape of cost the other two listings already pay for the same signal, and only paid at all when `h.search` is configured.
