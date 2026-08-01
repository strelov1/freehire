# Source ingest conventions

## Scope
Source ingest: board list, provider registry, board-file parsing/validation, per-board health sidecar, and related job-lifecycle mechanics.

## Always true
- **One file per provider** under `sources/` (`sources/<provider>.yml`) plus a mixed `sources/custom.yml`. Each entry is `company` + `board`, taking the file-name provider unless it names its own.
- **`sources.All` maps each `provider` string to a registered adapter** (`Source` interface) over a shared HTTP client. Adding a platform is a new adapter + one line in `sources.All`.
- **`cmd/ingest` processes one board file per run** (path as first argument or `SOURCES_FILE`). It validates every entry against the registry and **fails fast** — a misconfigured board never starts a run.
- **Run-once-and-exit worker** meant for cron (one schedule per file, so providers crawl independently). No long-lived process.
- **Adapters are read-only over public ATS JSON APIs.** Per-board crawl is independent: one failing board is counted (`stats.Failed`) but does not abort the rest.
- **Sources are keyless by default.** The exceptions are `usajobs` (`USAJOBS_API_KEY`), `reed` (`REED_API_KEY`) and `whatjobs` (`WHATJOBS_PUBLISHER_ID`). The credential lives in the env, never in a board file.
- **`All` is the crawl registry, `Taxonomy` is the classification one — the credential gates only the first.** `sources.All(client)` omits a keyed provider whose variable is unset, so its board file fails config validation before any request goes out. `sources.Taxonomy()` (`All(nil)`, no transport, never Fetch through it) is **total**: what kind of source a provider is cannot depend on whether this host can crawl it. Everything that classifies rather than crawls — `FilterableProviders`, `ProviderKind` on the status page, `AggregatorProviders` in `cmd/reindex` and `cmd/ghost-crosscheck` — reads `Taxonomy`. Conflating the two silently reclassified `whatjobs`, a reseller of first-party ATS postings, as an ATS on every keyless host, so none of its copies were suppressed.
- **A board is not always a tenant.** For `hh` it is a `professional_role`, for `trudvsem` an OKATO region code, for `whatjobs` a SEARCH KEYWORD. Such a provider is an `aggregator` (many employers, company read per posting) but NOT `boardless` — the board is what selects the slice to crawl.
- **`sweepGrace` widens the unseen sweep for a slice-crawled provider.** `whatjobs` reads at most 40 pages of a keyword from a feed thousands of pages deep, so a posting that drifts past that depth reads as unseen; on the 48-hour default it would be closed and reopened repeatedly, each cycle writing a phantom removal into `job_daily_stats`. The adapter declares 14 days instead (`sources.SweepGraceWindows` → `cmd/ingest`'s `sweepWindowFor`). Sound only where liveness cannot be probed — anything verifiable should close on evidence.
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

**WhatJobs FeedAPI traps** (its documentation is wrong in several places; all of the below was verified against the live API):

- A `/` in the `user_agent` query value makes the edge redirect with the value corrupted (`Mozilla/5.0` → `Mozilla%215.0`), which is why every code sample in the vendor's docs fails. The adapter sends no `user_agent` at all.
- `limit` above 50 is silently clamped, and `limit=1` **with** a keyword returns an empty `data` with `per_page: 0`. Worse, the feed post-filters duplicates *after* selecting a page, so a request for 50 routinely returns 44 while more pages remain — **a short page is not the last page**, and the same posting can appear on several pages of one keyword.
- Pagination dies past roughly 2000 pages regardless of the `total` reported (594k), so no keyword is ever exhaustible; keep board keywords narrow enough to fit the page budget.
- `snippet` is the FULL description HTML, not the highlighted excerpt the docs describe. The documented `onmousedown` field does not exist.
- `salary` is always `"0.000000 - 0.000000"`, `job_type` always `""`, `logo` always `null` — all three are dropped rather than stored.
- `age`/`age_days` measure the record's age in the reseller's index, not the posting date (postings from unrelated companies share one value), so `PostedAt` is left nil.
- An invalid publisher answers **410** (docs claim 422); `unique_id` does not deduplicate despite the docs.
- No country field, and its cities collide with foreign ones (London is Ohio, Vienna is Virginia) — the account's country is stated by the adapter, since the publisher id is per-country.

## Limitations
- No versioned migration runner for `board_health` migration (`0006_board_health.sql`) — apply to prod manually before deploying.
- The ingest sweep has a trade-off: a missed run can leave an orphan open until a future reconcile; the change window is sized wide enough to absorb a skipped cron.
- Self-closing sources: a missed `removed` event from the feed can leave a vacancy open until the next reindex.
