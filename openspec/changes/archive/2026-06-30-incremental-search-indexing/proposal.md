## Why

New and edited vacancies only become searchable after the next full batch reindex
(`cmd/reindex`, every 6h), so a freshly ingested posting — or an edit such as a
retitle of an existing one — is invisible in `/jobs` and `/jobs/search` for up to
six hours. Ingest already holds the persisted job row in hand; pushing the
new/changed rows straight to the live index closes the latency gap from hours to
one crawl cycle, which is the indexing path Meilisearch itself recommends
(incremental `addDocuments` into the live index; reserve a full rebuild for
settings/schema changes).

## What Changes

- The ingest worker pushes each crawl's **new or content-changed** open jobs to
  the live Meilisearch facet index, as a batch, right after they are persisted —
  so they are searchable within one crawl cycle instead of after the next 6h
  reindex.
- The job write path (`UpsertJob`) gains a deterministic `content_hash` over the
  indexed fields and reports whether a write **inserted** a row or **changed** its
  indexed content, so only rows whose searchable representation actually changed
  are re-pushed (an upsert that merely bumps `last_seen_at` is not).
- Incremental indexing is **best-effort**: a search-index failure is logged and
  never fails the ingest run. The existing batch reindex stays the source of
  truth and reconciler (compaction, settings, and dropping closed-job documents).
- Indexing targets only the **facet/keyword** production index; the semantic
  index keeps its own separate schedule.
- The behavior is wired only when the search engine is configured for the ingest
  worker; without it, ingest runs exactly as before (e.g. local dev).

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `job-search`: add a requirement that the live index is kept fresh incrementally
  — new and content-changed open jobs are indexed at ingest time, between batch
  reindexes, without waiting for the scheduled full rebuild.
- `source-ingest`: the normalized write path computes a content hash and reports
  whether a write inserted a row or changed its indexed content; the ingest
  command uses that signal to push new/changed jobs to the search index,
  best-effort, after they are persisted.

## Impact

- **Schema:** new `jobs.content_hash` column (migration). No data migration —
  the hash is populated on the next upsert of each row; a NULL hash simply means
  "changed" on the first touch.
- **DB access:** `UpsertJob` query gains the hash column and a `RETURNING`
  signal (`inserted`, `changed`); regenerate sqlc.
- **Code:** `cmd/ingest` (wire the search client + a batched indexer into the
  store), `internal/pipeline` (carry the per-write inserted/changed signal
  without coupling the pipeline to `db`/`search`), `internal/search` (already has
  `IndexJobs`; a batching buffer is added on the ingest side).
- **Ops:** ingest cron entries now need `MEILI_URL`/`MEILI_MASTER_KEY` in the
  ingest container env (already present in the prod compose) to enable indexing;
  absent, ingest no-ops the indexing step.
- **Non-goals:** closures stay on the batch-reindex reconciler (a job closed by
  the post-run sweep or the liveness probe leaves the index on the next reindex,
  unchanged from today); the semantic index is untouched.
