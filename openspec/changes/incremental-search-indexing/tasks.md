## 1. Schema and write-path signal

- [x] 1.1 Add migration: `jobs.content_hash text` (nullable, additive — no backfill).
- [x] 1.2 Define the indexed-content hash in Go (next to `pipeline.Job` / the fields
  `search.FromJob` reads): a deterministic SHA-256 over the canonical set of
  searchable fields. Add a unit test pinning that every field the search document
  uses is covered by the hash.
- [x] 1.3 Update the `UpsertJob` query: accept `content_hash`, and `RETURNING`
  `(xmax = 0) AS inserted` plus `(<old_hash> IS DISTINCT FROM <new_hash>) AS changed`
  (capture the pre-update hash via CTE). Regenerate sqlc; `go build ./...`.

## 2. Store reports inserted/changed

- [x] 2.1 Thread the computed hash into `dbStore.Save`'s `UpsertJobParams` and read
  back the `inserted`/`changed` flags.
- [x] 2.2 Decide reindex-worthiness in the store: a write needs indexing when it is
  open AND (`inserted` OR `changed`). Cover with a store-level test (insert → yes,
  edited → yes, last-seen-only refresh → no).

## 3. Batched, best-effort indexer

- [x] 3.1 Add a batching indexer (on the `cmd/ingest` side) that buffers
  `search.JobDocument`s, flushes at a chunk-size threshold and on an explicit final
  `Flush`, pushes via `search.Client.IndexJobs`, and is mutex-guarded for concurrent
  `Save`s. Flush copies-and-clears under the lock, pushes outside it.
- [x] 3.2 Make flush errors best-effort: log and swallow; never propagate to `Save`
  or the run. Test that a failing push does not error the store/run.
- [x] 3.3 In `dbStore.Save`, after the commit, convert reindex-worthy rows via
  `search.FromJob` and enqueue them into the indexer.

## 4. Wire the worker

- [x] 4.1 In `cmd/ingest/main.go`, construct the search client + indexer only when
  `MEILI_URL` + master key are configured; inject into the store. Absent → nil/no-op
  indexer, ingest unchanged.
- [x] 4.2 Call the indexer's final `Flush` after `runner.Run` (and before/independent
  of the sweep), logging the flushed/failed counts. Ensure exit code reflects only
  crawl/save/sweep outcomes, not indexing.

## 5. Verify

- [x] 5.1 `go build ./... && go vet ./... && go test ./...` green.
- [x] 5.2 Integration check (testcontainers/local stack): ingest a new job and an
  edited job, confirm both appear in the live facet index without a reindex; confirm
  a no-op re-ingest pushes nothing.
- [x] 5.3 Update `cmd/ingest` package doc and the source-ingest mention in
  `CLAUDE.md`/`AGENT.md` to note the incremental-index side-effect and its env gating.
