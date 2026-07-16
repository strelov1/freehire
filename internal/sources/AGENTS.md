# Source ingest conventions

## Scope
Source ingest: board list, provider registry, board-file parsing/validation, per-board health sidecar, and related job-lifecycle mechanics.

## Always true
- **One file per provider** under `sources/` (`sources/<provider>.yml`) plus a mixed `sources/custom.yml`. Each entry is `company` + `board`, taking the file-name provider unless it names its own.
- **`sources.All` maps each `provider` string to a registered adapter** (`Source` interface) over a shared HTTP client. Adding a platform is a new adapter + one line in `sources.All`.
- **`cmd/ingest` processes one board file per run** (path as first argument or `SOURCES_FILE`). It validates every entry against the registry and **fails fast** — a misconfigured board never starts a run.
- **Run-once-and-exit worker** meant for cron (one schedule per file, so providers crawl independently). No long-lived process.
- **Adapters are read-only over public ATS JSON APIs.** Per-board crawl is independent: one failing board is counted (`stats.Failed`) but does not abort the rest.
- **Sources are keyless by default; `usajobs` is the lone exception.** USAJobs Search API requires `Authorization-Key` header — `sources.All` registers it only when `USAJOBS_API_KEY` is set in the environment. The key lives in the env, never in a board file.
- **Dedup key is `jobs.UNIQUE (source, external_id)`.** `UpsertJob` is `ON CONFLICT` on it.
- **`sources.SelfClosingProviders`** lists providers whose adapters implement the `selfClosing` marker — they emit `Job{Removed: true}` for taken-down postings and are excluded from the unseen-job sweep.
- **Board health table holds ONLY runtime state** — the board catalog stays in YAML (git); a stale row for a removed board is inert.

## How it works

**Board registry:** each adapter implements the `Source` interface and speaks a common normalized `Job` shape. `sources.All` is the registry map. Adapters are in `internal/sources/` — one `.go` file per provider (`greenhouse.go`, `lever.go`, `ashby.go`, …) with matching `_test.go` files. `config.go` handles board-file parsing and validation.

**Ingest pipeline:** `cmd/ingest` loads the board file, validates every entry, then delegates to the `pipeline.Runner` which fetches each board once, normalizes postings, and `UpsertJob`s them (idempotent on the dedup key). New postings are enqueued into `enrichment_outbox` in the same transaction (transactional-outbox).

**Per-board health:** `board_health (provider, board)` tracks `consecutive_failures`, `cooldown_until`, `last_error`, timestamps, `last_ingested_count`. The `pipeline.Runner` uses an optional `BoardHealth` port (nil = feature off). It **skips a board whose `cooldown_until` is in the future** (counted `Cooled`, not `Failed`). Backoff (`pipeline.CooldownFor`): no cooldown below **3** consecutive failures, then `6h·2^(f-3)` capped at **24h**. A success self-heals. The backoff is Go-owned and unit-tested.

**Incremental search indexing:** `cmd/ingest` pushes each crawl's **new or content-changed** open jobs straight to the live facet index, batched, after persistence. The change signal is `jobs.content_hash` returned by `UpsertJob` as `inserted`/`changed`. The push is best-effort — search-engine failure is logged, never fails the run. Wired only when the worker has `MEILI_MASTER_KEY`. Full reindex (`cmd/reindex`) stays the source of truth — it owns settings, compaction, and removing closed-job documents.

**Proxy egress (opt-in, IP-blocklisted providers):** some ATS edges IP-blocklist the prod datacenter IP (e.g. eightfold 403s every prod-IP request while a residential IP is served). `SOURCES_PROXY_URL` (form `http://user:pass@host:port`) routes only the providers in the `proxiedProviders` allowlist through that egress proxy; everything else stays on the direct IP. Unset = no-op; set-but-invalid fails the run at startup. `cmd/ingest` calls `sources.ApplyProxyEgress(registry)` after `All`. The proxy endpoint + credentials live entirely in env — nothing is hardcoded. **SSRF caveat:** on the proxied path the guarded dialer vets the *proxy's* IP, not the ultimate target (the proxy resolves that), so `proxiedProviders` must list only trusted, fixed-host providers — never the link-following/liveness paths, which keep the direct target-guarded client.

**Link-following (`internal/linksource/`):** resolves a single outbound job-detail URL into a vacancy under the destination's own identity. A `LinkSource` adapts a single detail page (unlike `sources` which adapts a whole ATS board). Matching by link host. The resolved job dedups against the same posting if another source also has it.

**Job lifecycle — soft-close via `closed_at`:**
1. **Ingest sweep** (`CloseUnseenJobs`): post-run sweep closes a provider's jobs unseen for 48h. A reappearing posting reopens via the upsert. Self-closing sources are excluded.
2. **Stream-driven self-close** (`CloseJobBySourceExternalID`): self-closing sources emit `Job{Removed: true}`; the pipeline routes this to the Store's optional `closer`.
3. **Liveness probe** (`cmd/liveness`): URL-probes orphan jobs from non-board sources. Closes after two consecutive `expired` reads (the `liveness_strikes` counter).

**Telegram ingest** is a two-stage queue (crawl then LLM-extract): `cmd/tg-ingest` crawls `sources/telegram.yml` channels into `telegram_posts`; `cmd/tg-extract` drains via the LLM. Both are run-once-and-exit cron workers.

## Limitations
- No versioned migration runner for `board_health` migration (`0006_board_health.sql`) — apply to prod manually before deploying.
- The ingest sweep has a trade-off: a missed run can leave an orphan open until a future reconcile; the change window is sized wide enough to absorb a skipped cron.
- Self-closing sources: a missed `removed` event from the feed can leave a vacancy open until the next reindex.
