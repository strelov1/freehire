## Context

`/jobs` and `/jobs/search` are served from Meilisearch. The only write path from
Postgres to Meilisearch today is `cmd/reindex`, which rebuilds a fresh facet
index from the whole table and atomically swaps it over the live one, scheduled
every 6h (`15 */6 * * *`). Ingest (`cmd/ingest` → `internal/pipeline`) writes only
to Postgres via `UpsertJob`. Consequently a newly ingested or edited posting is
invisible to search for up to six hours.

The ingest store already holds what indexing needs: `dbStore.Save`
(`cmd/ingest/store.go`) calls `UpsertJob` (`:one`), which returns the persisted
`db.Job` — exactly the input `search.FromJob(db.Job)` turns into a
`search.JobDocument`. `search.Client.IndexJobs` already pushes documents into the
live facet index. The missing piece is (a) knowing which writes actually need
re-indexing, and (b) a batched, best-effort push wired into the ingest worker.

Constraints worth preserving:
- `internal/pipeline` is deliberately decoupled from `internal/db` and
  `internal/search` (it imports only `jobderive`/`sources`/`worker`). Indexing
  needs the `db.Job` row, which only the store sees, so indexing belongs on the
  `cmd/ingest` store layer, not in the pipeline.
- `UpsertJob` runs `ON CONFLICT DO UPDATE` on **every** crawl for **every** job
  (it always bumps `last_seen_at`/`updated_at`). Indexing every returned row would
  re-push the entire catalogue (millions of docs) hourly — the exact anti-pattern
  Meilisearch warns against (each batch triggers re-index work on a large index).
- `--since` reindex is already defeated because `updated_at` advances every crawl,
  so time is not a usable "changed" signal.

## Goals / Non-Goals

**Goals:**
- New and content-changed open jobs become searchable within one crawl cycle.
- Only rows whose indexed representation changed are pushed (no full-catalogue
  re-push).
- Indexing is best-effort: never fails or slows correctness of ingest.
- Reuse existing building blocks (`search.FromJob`, `Client.IndexJobs`); keep the
  pipeline decoupled from db/search.

**Non-Goals:**
- Removing closed-job documents incrementally. Closures (post-run sweep, liveness
  probe) continue to be reconciled by the batch reindex, unchanged from today.
- The semantic index. It keeps its own schedule.
- Replacing the batch reindex. It stays as reconciler/compaction/settings owner.
- Per-job synchronous push. Pushes are batched.

## Decisions

### 1. Detect changed content via a `content_hash` column + `RETURNING` signal

Add `jobs.content_hash text` (nullable). `UpsertJob` computes the hash of the
indexed fields and stores it; the query returns two booleans:

- `inserted` via `(xmax = 0)` — Postgres's standard insert-vs-update tell.
- `changed` via comparing the existing row's hash against the incoming hash.

The comparison must read the **pre-update** hash. In `ON CONFLICT DO UPDATE`, an
unqualified column in the conflict action refers to the existing row and
`EXCLUDED` to the proposed insert, but `RETURNING` sees post-update values. So the
query captures the old hash before overwriting it — either by selecting it in a
CTE keyed on `(source, external_id)` and exposing it in `RETURNING`, or by
guarding the hash update and reading back. The chosen mechanism is settled during
implementation against sqlc; the contract is: `RETURNING ... , (xmax = 0) AS
inserted, (<old_hash> IS DISTINCT FROM <new_hash>) AS changed`. A NULL old hash
(first touch after the migration) is `DISTINCT FROM` any value, so legacy rows
report changed on their next ingest and self-heal into the hash regime.

The hash is computed in Go (e.g. SHA-256 over the canonical concatenation of the
fields that form the search document) and passed as a parameter, so the
"indexed fields" definition lives next to `search.FromJob`/`pipeline.Job` rather
than in SQL.

*Alternative considered — insert-only (`xmax = 0`):* simpler, no migration, but
misses edits to existing rows (e.g. the observed CookUnity retitle), which the
user explicitly wants covered. Rejected.

*Alternative considered — compare every indexed column in SQL:* no extra column,
but bloats the query, duplicates the "what is indexed" definition into SQL, and
re-checks on every field change site. A single hash column is the smaller,
cohesive contract.

### 2. Indexing lives on the `cmd/ingest` store layer, behind a small interface

The pipeline's `Store.Save` stays `(ctx, Job) error` for the common path, but the
`inserted/changed` signal and the persisted row must reach an indexer. Options
weighed:

- **Decorate the store / accumulate in the worker (chosen).** `dbStore.Save`
  already has the persisted `db.Job` and now the `inserted/changed` flags. When a
  write is inserted-or-changed and the job is open, it converts the row to a
  `search.JobDocument` and hands it to a batching `indexer` held by the store. The
  pipeline is untouched; the seam is entirely inside `cmd/ingest`.
- Add an `Indexer` to `pipeline.Runner` — rejected: forces the pipeline to import
  `db`/`search` (the doc needs the db row), breaking its decoupling.

### 3. Batch boundary: buffer across the run, flush in fixed chunks

The indexer buffers documents and flushes when the buffer reaches a chunk size
(e.g. ~1000) and once more at run end. This yields few, fat batches (better for
Meilisearch than many tiny per-board pushes) and needs no per-board lifecycle
signal from the pipeline. Because saves run concurrently (board pool of 8, plus
the streaming path emits under a mutex), the buffer is mutex-guarded. Within one
run the visibility delay for a new job is at most "until its chunk flushes" —
seconds-to-minutes, versus 6h today.

*Alternative considered — flush per board:* would need a board-boundary callback
threaded through the pipeline; rejected as more coupling for negligible gain,
since Meilisearch also auto-batches server-side.

### 4. Best-effort, engine-optional

The indexer is constructed only when the worker has `MEILI_URL` + master key; the
prod ingest containers already carry them. Absent, the store holds a nil/no-op
indexer and ingest behaves exactly as before. Every flush error is logged and
swallowed — the batch reindex reconciles any missed push, and ingest's exit code
reflects only crawl/save/sweep outcomes.

## Risks / Trade-offs

- **[Index drifts from Postgres if pushes silently fail]** → Best-effort by
  design; the 6h batch reindex is the reconciler and corrects any miss. Failures
  are logged for visibility.
- **[Closed jobs linger until the next reindex]** → Same as today's behavior for
  closures; explicitly a non-goal. Incremental path only adds/updates open-job
  docs.
- **[Hash false-negatives hide a real edit]** → The hash must cover every field
  `search.FromJob` reads; a unit test pins "indexed fields ⊆ hashed fields" so a
  future searchable field added without hashing is caught. Worst case still
  self-heals at the next reindex.
- **[Extra Meilisearch write load during ingest]** → Bounded: only inserted/changed
  rows are pushed (typically dozens–hundreds per run, not the full catalogue), in
  fat batches; the semantic (embedding) index is untouched so no per-doc embed
  cost is incurred on this path.
- **[Concurrency on the shared buffer]** → Mutex-guarded; flush copies-and-clears
  under the lock then pushes outside it to avoid holding the lock across I/O.

## Migration Plan

1. Ship the `content_hash` migration (additive, nullable — no backfill needed;
   rows hash themselves on their next upsert).
2. Deploy the new ingest binary. With the engine configured, new/changed jobs
   start flowing into the live index immediately; legacy rows report `changed`
   once (NULL hash) and settle.
3. The batch reindex schedule is unchanged and keeps reconciling.

**Rollback:** revert the ingest binary; the column is harmless if left in place
(ignored by the old code path). The live index stays correct because the batch
reindex continues to rebuild it from Postgres.

## Open Questions

- Exact `RETURNING`/CTE shape for the pre-update hash under sqlc — resolved in the
  first task against the generator, not blocking the design.
- Chunk size constant (start ~1000, a `const` like `reindexBatchSize`; promote to
  config only if it needs tuning).
