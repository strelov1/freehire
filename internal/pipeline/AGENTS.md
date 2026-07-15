# Pipeline conventions

## Scope
The ingest pipeline runner — fetch → normalize → dedup → upsert — and the incremental search indexing and job-lifecycle close that follow.

## Always true
- `jobs.UNIQUE (source, external_id)` is the dedup key; `UpsertJob` is `ON CONFLICT` on it.
- `cmd/ingest` processes one board file per run (path as the first argument — cron passes it — or `SOURCES_FILE`); it is a run-once-and-exit worker, never a long-lived process.
- Validate every entry against the registry and fail fast — a misconfigured board never starts a run.
- Adapters are read-only over public ATS JSON APIs; the per-board crawl is independent, so one failing board is counted (`stats.Failed`) but does not abort the rest.
- Sources are keyless by default; `usajobs` is the lone exception — `sources.All` registers it only when `USAJOBS_API_KEY` is set. The key is a secret that lives in the env, never in a board file.
- After the run, the stale-job sweep runs per provider, and only for providers that ingested at least one job (so a total crawl outage can't mass-close a catalogue).
- `board_health` holds ONLY runtime state — the set of boards and their cadence stay in the YAML board files (git owns the catalog); a stale row for a removed board is inert.
- A board whose `cooldown_until` is in the future is skipped before touching its adapter (counted `Cooled`, not `Failed`).
- Backoff: no cooldown below 3 consecutive failures, then `6h·2^(f-3)` capped at 24h — never permanent, so a success self-heals.
- A fetch/registry failure counts for board health; a per-job save skip does not. A streaming board with partial progress is a success.
- The incremental search-index push is best-effort — a search-engine failure is logged and never fails the run.
- The incremental push is wired only when the worker has `MEILI_MASTER_KEY` (absent, ingest runs unchanged).
- The incremental doc is built from the persisted row (`search.FromJob`), so a re-ingested already-enriched job keeps its enrichment facets.
- `cmd/tg-extract` shares `UpsertJob` but is not wired for incremental indexing (its jobs reconcile via reindex).
- A job is open while `closed_at IS NULL`; closing is a soft state, never a delete.
- Self-closing sources are excluded from the unseen sweep (`sources.SelfClosingProviders`).

## How it works
Boards live under `sources/` as one file per provider (`sources/<provider>.yml`) plus a mixed `sources/custom.yml`; each entry is `company` + `board`, taking the file-name provider unless it names its own. `sources.All` maps each `provider` string to a registered adapter, all speaking the `Source` interface over a shared HTTP client. The `pipeline.Runner` fetches each board once, normalizes postings, and `UpsertJob`s them (idempotent on the dedup key, so re-running is safe). Adding a company is one entry in the provider's board file; adding an ATS platform is a new adapter in `internal/sources` plus one line in `sources.All`.

`board_health (provider, board)` remembers each board's last outcome (`consecutive_failures`, `cooldown_until`, `last_error`, timestamps, `last_ingested_count`) so a repeatedly-failing board backs off instead of being hammered every run. The `pipeline.Runner` records each crawl's board-level outcome through an optional `BoardHealth` port (nil = feature off, so unit tests and non-DB callers are unaffected). The backoff (`pipeline.CooldownFor`) is Go-owned and unit-tested. The Runner logs a per-run summary of unhealthy boards; the table is directly SQL-queryable. This is Slice-0 over the existing cron (no daemon/queue).

`cmd/reindex` rebuilds a fresh facet index and swaps it in on a schedule (hours), so without help a newly ingested or edited posting is unsearchable until the next rebuild. To close that gap, `cmd/ingest` pushes each crawl's new or content-changed open jobs straight to the live facet index, batched, after they are persisted — searchable within one crawl cycle. The change signal is a `jobs.content_hash` (a fingerprint of the indexed fields, `internal/jobhash`) the `UpsertJob` write returns as `inserted`/`changed`: an upsert that only bumps `last_seen_at` reports neither and is not re-pushed (so the whole catalogue isn't re-indexed every crawl). The full reindex stays the index's source of truth: it owns settings, compaction, and removing closed-job documents (the incremental path only adds/updates open jobs; closures still reconcile on the next reindex, unchanged).

Job lifecycle is one soft-close column (`closed_at`) written by three mechanisms. (1) Ingest sweep (`CloseUnseenJobs`): for board sources, `UpsertJob` stamps `last_seen_at` every crawl and the post-run sweep closes a provider's jobs unseen for 48h; a reappearing posting reopens via the upsert. (3) Stream-driven self-close (`CloseJobBySourceExternalID`): a self-closing source (a streaming aggregator like `jobtech`/Arbetsförmedlingen that consumes an incremental change feed) emits a `Job{Removed: true}` for a posting its feed reports taken down; `pipeline.ingestStream` routes that to the Store's optional `closer` (the ingest `dbStore`), closing by `(source, external_id)`. (2) Liveness probe (`cmd/liveness`): jobs from non-board sources are never re-crawled, so the sweep can't reach them; the liveness worker URL-probes those orphans and closes dead ones.

## Limitations
- The `board_health` migration (`0006_board_health.sql`) has no versioned runner, so apply it to prod manually before deploying (per the migrations gotcha).
- A missed self-closing source run can leave an orphan open until a future reconcile; the change window is sized wide enough to absorb a skipped cron.
