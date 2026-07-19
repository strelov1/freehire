# Job lifecycle conventions

## Scope
The open/closed state of a job row, the three mechanisms that write `closed_at`, and the filtering semantics that depend on it.

## Always true
- A job is open while `closed_at IS NULL`. Closing is a soft state, never a delete.
- A closed row keeps its `public_slug`, enrichment, and `user_jobs` references, and reopens for free.
- List, search, and company surfaces filter `closed_at IS NULL`. Detail still serves a closed job (with `closed_at`) so links and history don't break.
- `UpsertJob` stamps `last_seen_at` on every crawl; the post-run sweep closes a provider's jobs unseen for 48h.
- A reappearing posting reopens via the upsert.
- Self-closing sources (`jobtech`, etc.) are excluded from the unseen sweep — the feed's `removed` events are the authoritative close signal.
- The liveness worker closes only on positive evidence (two consecutive `expired` reads) and never reopens.

## How it works
Closing is a soft state on one column (`closed_at`) written by three independent mechanisms, each covering a gap the others can't reach.

**(1) Ingest sweep** (`cmd/ingest`, `CloseUnseenJobs`): for board sources, `UpsertJob` stamps `last_seen_at` every crawl. The post-run sweep closes a provider's jobs unseen for 48h — if a posting drops off a board we crawl, it closes. A reappearing posting reopens via the upsert (the `ON CONFLICT` path clears `closed_at`). The sweep runs per provider, and only for providers that ingested at least one job, so a total crawl outage can't mass-close a catalogue. It is further scoped to the `company_slug`s the run actually crawled, so a partial or targeted run closes only the companies it saw — a deliberate under-close that *leaks the last postings of a company that vanished from the feed entirely* (its slug drops out of the crawled set), since the scope can't tell "company gone" from "company not reached this run".

**(1b) Full-catalogue source sweep** (`CloseUnseenJobsBySource`): a *full-catalogue* aggregator (`sources.fullCatalog` marker, e.g. `habr_career`) lists its whole catalogue every run, so an unseen job is genuinely gone — including the vanished-company case (1) leaks. For such a provider the sweep drops the `company_slug` scope and closes by source alone. Sound **only** because a full-catalogue adapter errors a *truncated* crawl instead of returning it as a partial success: `cmd/ingest` gates the source-scoped close on a zero-`Failed` run (`sweepBySource`), so a mid-listing failure (`Failed>0`) falls back to the safe company-scoped close rather than mass-closing every posting past the failed page. `geekjob` is a full-catalogue aggregator too but is *not* yet marked — its adapter still salvages a truncated walk as partial success, which must be hardened to error first.

**(3) Stream-driven self-close** (`CloseJobBySourceExternalID`): a *self-closing* source (a streaming aggregator like `jobtech`/Arbetsförmedlingen that consumes an incremental change feed) emits a `Job{Removed: true}` for a posting its feed reports taken down. `pipeline.ingestStream` routes that to the Store's optional `closer` (the ingest `dbStore`), closing by `(source, external_id)`. Such a source implements the `selfClosing` marker and is excluded from the (1) unseen sweep (`sources.SelfClosingProviders`): it re-reports only changed ads, so the sweep's `last_seen_at` cutoff would wrongly close every still-open ad it did not touch. Trade-off: a missed run can leave an orphan open until a future reconcile; the change window is sized wide enough to absorb a skipped cron.

**(2) Liveness probe** (`cmd/liveness`): board sources are not the whole catalogue — jobs from sources not in the `sources.All` registry (manual/`resolve-url` imports and the like) are never re-crawled, so the sweep can't reach them. (Aggregators like `habr_career`/`geekjob` *are* registered providers, swept by (1)/(1b) and excluded from the probe; `telegram` is registered-excluded too via `unprobableSources` because its URL outlives the vacancy.) The liveness worker URL-probes those orphans, classifies the page via `internal/liveness` (pure heuristics — HTTP 404/410, error/listing redirect, curated EN/DE/FR closed-posting phrases, or near-empty content — no browser, no LLM), and closes a job after two consecutive `expired` reads (the `liveness_strikes` counter; any healthy probe resets it). It closes only on positive evidence and never reopens, biasing toward under-closing (an orphan has no re-ingest to reopen it). Run-once-and-exit, cron-scheduled.

## Limitations
- A missed liveness cron run leaves orphans open longer; no reconciliation beyond the next run.
- The liveness probe uses pure heuristics (no browser, no LLM) — a posting that returns a 200 with a "position filled" message in a language or phrasing not in the curated set stays open.
- Self-closing sources trade missed-run safety for feed-accuracy: a skipped cron leaves orphans open until the next run's change window catches up.
