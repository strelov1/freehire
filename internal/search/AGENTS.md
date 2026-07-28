# Search conventions

Meilisearch-backed keyword and hybrid search over jobs and companies. The package doc in
`client.go` explains the index topology; this file covers what the code can't tell you.

## Always true

- **Meilisearch has ONE serial task queue.** Two rebuilds do not run concurrently — the
  second queues behind the first and looks like a hang while the engine is genuinely busy.
  Before triggering any rebuild, check `ps aux | grep reindex` and
  `GET /tasks?statuses=processing`. Never stack `reindex-companies` with `make reindex`, and
  never stack anything with a `--semantic` pass.
- **Killing the reindex client does not cancel enqueued Meili tasks.** To actually stop:
  `POST /tasks/cancel?indexUids=<uid>&statuses=enqueued,processing`. That cancelation itself
  queues, and it is irreversible — don't fire it on an unconfirmed diagnosis.
- **A failed rebuild leaves an orphan `*_rebuild` index.** `Rebuild.Promote` drops the old
  data only *after* a successful swap, so an aborted run leaves a full-size index on disk
  (~55 GB at catalogue scale). That has filled the production disk and put rebuilds into a
  death spiral: ENOSPC → orphan → less disk → ENOSPC. Reclaim with
  `DELETE /indexes/<uid>_rebuild`. `Rebuild.Prepare` also drops a leftover before starting.
- **Live reads are never affected by a rebuild** — the swap is atomic.
- Full rebuilds are **content_hash-incremental even at `scope=full`**: `cmd/reindex` scans
  every row but pushes only documents whose `content_hash` differs (`indexed=X skipped=Y`).
  A field absent from `jobhash.Of()` therefore never reaches the index on its own —
  **`is_tech` is deliberately not hashed**, so an is_tech-only flip is invisible to search
  until the document is pushed for some other reason. There is no `--force` flag; the only
  way to surface it immediately is a rebuild from empty.
- `SubmitJobs` submits **without awaiting** the Meili task (the ingest hot path — awaiting
  stalled board goroutines); `IndexJobs` awaits (the reindex path). Pick deliberately.
- `swapIndexes` calls `POST /swap-indexes` over raw HTTP, not the SDK: the pinned
  meilisearch-go always serializes a `rename` field that engine v1.13 rejects.
- Indexed descriptions are capped at `maxIndexedDescriptionRunes`; `maxTotalHits` is the
  count-honesty cap, **not** the pagination guard (that's `maxSearchWindow` in the handler).

## Adding a filterable attribute

Adding one creates a hard-500 window: the app emits the new filter the moment the image
rolls out, but the attribute only becomes filterable when the rebuild swaps in — ~26 min at
catalogue scale. Meili answers `invalid_search_filter` (400), and the handler maps any Meili
error to 500, so the whole filtered page breaks rather than degrading.

Either run the reindex **before** rolling out the app image, or push the new index settings
to the **live** index first (settings updates are cheap; documents lag, so results are stale
or empty — never a 500).

## Limitations

- A Meili filter error 500s the page instead of degrading. That's the robustness seam.
- `jobs_semantic` is built by a separate, much slower pass and is only queried when
  `SemanticRatio > 0`. Always scope a semantic rebuild (`--posted-within 30d`); a bare full
  embed of the whole catalogue takes hours and monopolizes the queue.
