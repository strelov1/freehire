# AGENTS.md

Guidance for AI agents working in this repository.

## Working principles

- **No overengineering, and no MVP shortcuts.** Hold the middle path: don't build infrastructure before there's a concrete need (note the seam for later instead), and don't ship quick-and-dirty or "for now" hacks. Build each feature correctly and idiomatically — neither gold-plated nor a placeholder.
- **MVP stage — keep the architecture fluid.** The current structure is not load-bearing legacy. When a new feature doesn't fit cleanly, prefer reshaping the affected part over bolting on an awkward special case.
- **Surgical changes.** Clean up what your change orphaned; leave pre-existing dead code alone. Prefer a library's intended API over a clever shim.
- **English only.** All code, comments, identifiers, docs, and commits.

## What this is

`freehire` ([freehire.me](https://freehire.me)) is an open-source IT job aggregator. Many source parsers feed a pipeline that normalizes jobs into one schema, deduplicates them, and enriches them with AI; served over an HTTP API with rich filters, consumed by a SvelteKit app under `web/`.

Stack: **Go + Fiber v2**, **PostgreSQL**, **sqlc**, **Meilisearch**, **Docker Compose**, **langchaingo**.

## Layout

`internal/<domain>/` — domain packages, the substantial ones carry their own AGENTS.md (see the table below).
`cmd/<name>/` — every binary is a **run-once-and-exit** worker except `cmd/server` and `cmd/mail-ingest` (a long-lived SES inbound daemon, `Restart=always`). The rest are cron-driven, not daemons; they need `DATABASE_URL` and exit non-zero on failure.

Non-obvious:

- `migrations/` — the source for **both** sqlc codegen and Postgres initdb. Never edit an applied migration; add a new file.
- `sources/` — YAML board files, not Go. One file per ATS provider, plus `custom.yml` and `telegram.yml`.
- `design-system/` — a separate pnpm package, sibling to `web/` and `extension/`, linked via
  pnpm's `link:../design-system` (`web/`) or npm's `file:../design-system` (`extension/`) — both
  are symlinks, not copies. **Install it before building either consumer** — neither package
  manager installs a linked/`file:` package's own dependencies for you.
- `extension/` — the browser extension (WXT + Svelte side-panel agent client), npm-managed
  unlike the rest of the JS in this repo. See [extension/AGENTS.md](extension/AGENTS.md).
- `internal/db/` — **generated**; edit `internal/db/queries/*.sql` and run `make sqlc`.
- `services/pii-filter` — a standalone service, not a Go package.

## Commands

```bash
make up / make down / make logs       # start / stop / tail app + postgres in Docker
make run / make psql / make sqlc      # run server on host / psql into DB / regenerate internal/db
make reindex                          # rebuild the Meilisearch jobs index from Postgres
go build ./...  &&  go vet ./...
go test ./...                             # unit tests (no external deps)
go vet -tags=integration ./...            # compiles the tagged tests — run before EVERY push
go test -tags=integration ./internal/db/  # queue integration tests (needs Docker; testcontainers)
golangci-lint run                         # .golangci.yml; CI only fails on issues new-from-main (ratchet)
```

**Before committing** any `*.go` file: `gofmt -w` those paths (`gofmt -l .` must print
nothing), then `go vet ./...` and `go test ./...`. Do not commit if they fail. Skip this
suite when the commit has no Go. Integration-tagged tests stay push-time (`go vet
-tags=integration ./...` before every push; the full tagged suite when behaviour changed).

**Pre-commit hooks:** [lefthook](https://github.com/evilmartians/lefthook) (`go install
github.com/evilmartians/lefthook@latest`, then `lefthook install` once per clone) runs
gofmt/vet/golangci-lint on staged Go files and eslint on staged files in web/, extension/,
design-system/ — the same ratchet policy as CI (only new issues fail the commit), so it
won't block on the pre-existing backlog.

**`go test ./...` compiles no `//go:build integration` file, and those files are not
confined to `internal/db`** — there are 187 of them across 20 packages, and `internal/handler`
holds 78, which call unexported constructors like `newCVHandlers`. A changed signature
therefore passes every command above except the `vet` line, then fails CI, which runs
`go test -tags=integration ./...` over the whole module. The vet line is the cheap guard:
seconds, no Docker. Run the full tagged suite when you change behaviour rather than a
signature.

Worker gotchas (`go run ./cmd/<name>`, all need `DATABASE_URL`; run `ls cmd/` for the full list):

- `migrate` — run **before** deploying code that reads new schema. `-baseline` records on-disk files without executing; a pre-runner database auto-baselines on first run.
- `ingest` — takes one board file: `go run ./cmd/ingest sources/<provider>.yml` (or `SOURCES_FILE`). `HYDRATION_RETRY_DAYS` widens the window (default 14 days, measured from `created_at`) during which a stored row with NO description is withheld from the seen-set so a hydrating adapter re-attempts its detail fetch. Set it only for a deliberate repair run — `HYDRATION_RETRY_DAYS=365 ingest sources/hh.yml` re-offers every body-less row of that provider, one extra detail request each (hh's detail pages are ~1 MB), and expect to repeat it: a run that hits its timeout still persists what it hydrated and the fixed rows leave the set, so successive runs shrink the backlog. An unparseable or non-positive value fails the run rather than falling back, because a typo here would look exactly like a repair that found nothing.
- `enrich` / `tg-extract` — need `LLM_BASE_URL` / `LLM_API_KEY` / `LLM_MODEL`.
- `embed` / `search-drain` / `rollup-facets` / `reindex-companies` — need `MEILI_URL` / `MEILI_MASTER_KEY`. `search-drain` drains `search_outbox` (queued by `cmd/ingest`, atomically with each write) into the live facet index in batches — run it frequently (e.g. every 1-2 min); see [internal/searchdrain/AGENTS.md](internal/searchdrain/AGENTS.md).
- `reindex` — rebuilds the Meilisearch jobs index. **`REINDEX_DEDUP=1` additionally refreshes the duplicate markers** (role clusters, aggregator suppression, fuzzy collapse) before the rebuild; without it the rebuild uses the markers the last dedup run left. Off by default since 2026-08-16: aggregator suppression alone measured ~23h against a 12h unit timeout, so the run was cancelled mid-dedup and never reached the rebuild — 3 days with zero successful reindexes. Run the dedup invocation on its own, rarer schedule.
- `backfill-derive` — re-derives every deterministic column (facets, `role_fingerprint`, slugs) in one keyset pass; `BACKFILL_CONCURRENCY` tunes the pool. Follow with `REINDEX_DEDUP=1 make reindex` — that is what collapses newly-clustered reposts and unions their geography.
- `backfill-slug-folded` — one-off: fills `jobs.company_slug_folded` for the rows that predate the column (migration 0109). Chunked, paced, and idempotent — the chunk UPDATE is `IS DISTINCT FROM`-guarded, so a re-run writes nothing and stopping it mid-way is free. Until it completes the aggregator-suppression pass simply matches fewer rows, never wrong ones. Needs only `DATABASE_URL`.
- `capture-apply-form` — drains the apply-form capture queue: fetches each queued posting's application form from `greenhouse`/`ashby`/`workable`/`lever` and stores it in `apply_forms`. Needs nothing but `DATABASE_URL`; `APPLY_FORM_CONCURRENCY` (default 4) bounds how hard one run leans on a platform and `APPLY_FORM_MAX_PER_RUN` (default 5000) how much of the backlog it takes — the second matters because the first drain faces a ~185k backlog and an unbounded run would work for hours, which `Type=oneshot` turns into silently skipped timer firings. `recruitee` forms never reach this queue — its listing carries them, so ingest writes them directly.
- `hydrate-adzuna-description` — drains the Adzuna full-description capture queue (`cmd/ingest` enqueues eligible postings; see [internal/sources/AGENTS.md](internal/sources/AGENTS.md)). `ADZUNA_DESCRIPTION_MAX_PER_RUN` (default 500) is deliberately conservative — untested against Adzuna at real crawl-host volume. `seed-adzuna-description-queue` is the one-off companion that queues the pre-existing backlog; run it once, then let the cron drain handle the rest.
- `queue-metrics` — measures outbox depth, board-fleet health, and catalogue freshness and publishes them as Prometheus gauges via the node_exporter textfile collector. Needs `DATABASE_URL`, and is a **no-op that never opens the pool** without `PROM_TEXTFILE_DIR`. Read-only and lock-free by design — run it every minute; see [internal/worker/AGENTS.md](internal/worker/AGENTS.md) for the published names.
- **Nothing holds a flock** — no file locks in Go or in the systemd units; the word survives only in a few stale comments. What keeps a cron worker from stacking on itself is systemd: a `Type=oneshot` unit will not start a second instance while the first is active. That protects the TIMER path only, so a run started by hand has no lock at all. **Never stack `reindex-companies` with `make reindex`** — Meilisearch runs one serial task queue, so the second rebuild queues behind the first and looks like a hang, and a swap transiently holds ~2x the index's disk. Two workers additionally take a Postgres advisory lock (`cmd/liveness`, `cmd/ghost-crosscheck`); their keys are listed in `internal/migrate`.
- `prune` — the **only** hard-delete path. Dry-run by default; archives every removal to `pruned_jobs`.

## Module files

Each is self-contained and can be read independently.

| Area | Reference |
|---|---|
| **System architecture** (the high-level map: topology, repo layout, the three main flows) — start here | [docs/architecture.md](docs/architecture.md) |
| **HTTP handlers** (response shapes, error rendering, routes) | [internal/handler/AGENTS.md](internal/handler/AGENTS.md) |
| **SQL layer** (sqlc, queries, migrations) | [internal/db/AGENTS.md](internal/db/AGENTS.md) |
| **Search** (Meili index topology, rebuild swap, reindex hazards) | [internal/search/AGENTS.md](internal/search/AGENTS.md) |
| **Accounts** (identity resolution, seizure rule, mailed codes) | [internal/accounts/AGENTS.md](internal/accounts/AGENTS.md) |
| **Auth primitives** (JWT, API keys, cookie transport, middleware) | [internal/auth/AGENTS.md](internal/auth/AGENTS.md) |
| **OAuth sign-in** (provider registry, state cookie, identity resolution) | [internal/auth/oauth/AGENTS.md](internal/auth/oauth/AGENTS.md) |
| **Per-user job tracking** (view/apply/save/track, stages, /me/tracking) | [internal/userjob/AGENTS.md](internal/userjob/AGENTS.md) |
| **Job wire shape** (the single public projection of a job) | [internal/jobview/AGENTS.md](internal/jobview/AGENTS.md) |
| **Browser tools** (relays tool frames between agent harness and extension) | [internal/browsertools/AGENTS.md](internal/browsertools/AGENTS.md) |
| **Browser extension** (WXT + Svelte side-panel agent client, the other end of Browser tools) | [extension/AGENTS.md](extension/AGENTS.md) |
| **Source ingest** (board files, provider registry, validation) | [internal/sources/AGENTS.md](internal/sources/AGENTS.md) |
| **Pipeline** (Runner, dedup, UpsertJob, board health, search indexing) | [internal/pipeline/AGENTS.md](internal/pipeline/AGENTS.md) |
| **Apply-form capture** (ATS application forms, verbatim platform vocabulary, queue drain) | [internal/applyform/AGENTS.md](internal/applyform/AGENTS.md) |
| **Job fingerprints** (content_hash vs role_fingerprint vs RoleKey — which hash for which job) | [internal/jobhash/AGENTS.md](internal/jobhash/AGENTS.md) |
| **Cron worker plumbing** (Main/Bootstrap, exit codes, heartbeat, corruption-tolerant scans) | [internal/worker/AGENTS.md](internal/worker/AGENTS.md) |
| **Link resolution** (outbound job URL → destination's own identity) | [internal/linksource/AGENTS.md](internal/linksource/AGENTS.md) |
| **ATS board recognition** (URL → (source, board), shared conventions) | [internal/atsboard/AGENTS.md](internal/atsboard/AGENTS.md) |
| **Board contributions** (crowdsourced URL → (source, board) onboarding) | [internal/contribution/AGENTS.md](internal/contribution/AGENTS.md) |
| **Employee referrals** (offer/request marketplace, moderation, anonymity) | [internal/referral/AGENTS.md](internal/referral/AGENTS.md) |
| **Telegram** (crawl + LLM vacancy extraction) | [internal/telegram/AGENTS.md](internal/telegram/AGENTS.md) |
| **Company names** (real display names for slug-named companies) | [internal/companyname/AGENTS.md](internal/companyname/AGENTS.md) |
| **Enrichment** (Enrichment contract, LLM Provider; enums live in `internal/vocab`) | [internal/enrich/AGENTS.md](internal/enrich/AGENTS.md) |
| **Semantic embedding** (semantic_outbox, incremental chunked embeds into pgvector) | [internal/embed/AGENTS.md](internal/embed/AGENTS.md) |
| **Similar-jobs backfill** (precomputes `/similar`'s data via pgvector, no outbox) | [internal/similarjobs/AGENTS.md](internal/similarjobs/AGENTS.md) |
| **Facet-search drain** (search_outbox, incremental facet-index pushes, reconciler) | [internal/searchdrain/AGENTS.md](internal/searchdrain/AGENTS.md) |
| **In-app assistant** (turn loop, tool registry, presets, transcripts) | [internal/assistant/AGENTS.md](internal/assistant/AGENTS.md) |
| **Speech to text** (dictation into the composer, the filename rule, spend bounds) | [internal/speech/AGENTS.md](internal/speech/AGENTS.md) |
| **LLM client** (provider-agnostic wrapper, schema cache, streaming, attribution tags) | [internal/llm/AGENTS.md](internal/llm/AGENTS.md) |
| **LLM spend attribution** (per-user gateway credential, feature tags, fail-open) | [internal/llmkey/AGENTS.md](internal/llmkey/AGENTS.md) |
| **AI fit analysis** (three-stage LLM prompt-chain, score, verdict, stream) | [internal/matchanalysis/AGENTS.md](internal/matchanalysis/AGENTS.md) |
| **Job-match scoring** (deterministic CV-vs-vacancy score, the unverifiable rule) | [internal/cvmatch/AGENTS.md](internal/cvmatch/AGENTS.md) |
| **ATS-readiness score** (deterministic CV score, de-identified LLM review, delta) | [internal/atscheck/AGENTS.md](internal/atscheck/AGENTS.md) |
| **Experience bank** (durable employments + evidence atoms, provenance, retrieval) | [internal/experience/AGENTS.md](internal/experience/AGENTS.md) |
| **Résumé identity** (one stored CV per user, contact-block layers) | [internal/resume/AGENTS.md](internal/resume/AGENTS.md) |
| **Structured CV** (LLM parse of stored CV, stamp-and-compare) | [internal/resumeextract/AGENTS.md](internal/resumeextract/AGENTS.md) |
| **PII masking** (fail-closed CV→LLM redaction, reversible placeholders) | [internal/pii/AGENTS.md](internal/pii/AGENTS.md) |
| **CV rendering** (templates, fonts, previews) | [internal/cv/AGENTS.md](internal/cv/AGENTS.md) |
| **CV edits** (the only writer: path operations, revisions, undo, the evidence gate) | [internal/cvedit/AGENTS.md](internal/cvedit/AGENTS.md) |
| **Geography** (country/region codes, work-mode hint, dict-only vs hybrid) | [internal/location/AGENTS.md](internal/location/AGENTS.md) |
| **Skill tagging** (alias→canonical dictionary, jobs.skills facet) | [internal/skilltag/AGENTS.md](internal/skilltag/AGENTS.md) |
| **Seniority & category** (title→seniority/category, dict-only) | [internal/classify/AGENTS.md](internal/classify/AGENTS.md) |
| **View counts** (nginx access logs → per-job views) | [internal/viewlog/AGENTS.md](internal/viewlog/AGENTS.md) |
| **Ghost detection** (hedged posting-reality verdict, two evidence tiers, crosscheck) | [internal/ghost/AGENTS.md](internal/ghost/AGENTS.md) |
| **YC directory** (import-yc, curated facets, matching by former names) | [internal/ycdir/AGENTS.md](internal/ycdir/AGENTS.md) |
| **Company collections** (curated company tags, register datasets, reconciliation) | [internal/collections/AGENTS.md](internal/collections/AGENTS.md) |
| **Sentry error tracking** (backend, workers, frontend — env-gated) | [internal/observability/AGENTS.md](internal/observability/AGENTS.md) |
| **Mail stack** (Gmail + SES inbound → classify → link → stage advance) | [docs/agents/mail-stack.md](docs/agents/mail-stack.md) |
| **Notifications** (subscription digests, saved-job reminders, channels) | [docs/agents/notifications.md](docs/agents/notifications.md) |
| **Job lifecycle** (soft-close, ingest sweep, self-close, liveness probe) | [docs/agents/job-lifecycle.md](docs/agents/job-lifecycle.md) |
| **Company facets** (remote_regions vs yc_* curated facets) | [docs/agents/company-facets.md](docs/agents/company-facets.md) |
| **SPA sub-context** (SvelteKit, auth flow, API conventions) | [web/AGENTS.md](web/AGENTS.md) |
| **Design system** (pnpm package, tokens, Tailwind @source trap) | [design-system/AGENTS.md](design-system/AGENTS.md) |

## Conventions

- **Response shapes:** Lists: `{"data": ..., "meta": {...}}`; single items: `{"data": ...}`; errors: `{"error": msg}`
- **Dedup key:** `jobs.UNIQUE (source, external_id)` — `UpsertJob` is `ON CONFLICT` on it
- **Auth:** JWT in httpOnly cookie, same-origin, carrying the account's `token_version` so sessions are revocable. `RequireAuth` (cookie only) / `RequireAuthOrKey` (cookie or full-scope Bearer) / `RequireAuthOrScopedKey` (also admits a narrow key)
- **Email ownership:** `users.email_verified`; a password registration starts unverified and is confirmed by a mailed six-digit code. An unverified, password-backed account is **seized** (password cleared, sessions revoked, API keys deleted) when a provider-verified OAuth identity arrives for its address — the account-pre-hijacking defence
- **API keys:** Hashed at rest (SHA-256), scoped `full` or `cv`, and mintable only by an account with a verified address. Key management (create/list/revoke) and password change are cookie-only. A key does not carry the session generation, so a `token_version` bump does not revoke it — that is intentional for sign-out-everywhere and wrong for a takeover, so the seizure and the mailed-code password reset delete the rows in the same statement
- **Enrichment:** Queue-driven (`enrichment_outbox`), provider-agnostic LLM, `Sanitize` + `Validate` gate
- **Embeddings:** Queue-driven (`semantic_outbox`), incremental (`cmd/embed`) — pgvector-backed `job_semantic_chunks` plus a legacy single-vector column, no search index. Reconciled by bumping the embedder-model version string, which re-enqueues the whole catalogue through the existing staleness check
- **Catalogue scale:** Every public figure describing how big the catalogue is — `GET /api/v1/stats/catalog`, the jobs list's `meta.total`, the `/about` and `/open` strips — reads ONE snapshot published by `cmd/rollup-stats` (`internal/catalogstats`). Never count on a request path: `catalogstats.Load` takes no exact counter, so it cannot. A read never fails; a cold cache, an unreachable Redis or no cache at all degrades to the planner estimate with `exact: false`, which zeroes the figures that exist only in the database — pass `exact` through to whatever renders them, because a zero must not reach a page as if it were a measurement
- **Dictionaries:** All facet dictionaries are dict-only in production — never guess, emit nothing for unknowns
- **Job deletion:** The lifecycle only soft-closes; `cmd/prune` is the sole hard-delete path
- **In-app assistant:** a bounded tool-calling loop in-process (`internal/assistant`), streamed over SSE, open to every signed-in user. Tools act as the authenticated caller — no credential is minted for an agent
- **LLM spend attribution:** every model call made for a signed-in user goes out on that user's OWN gateway credential (`internal/llmkey` — minted lazily on first use, never shown to them) and carries a `feature:` tag. Work that belongs to nobody — enrichment, Telegram, embeddings — keeps the service credential, and a test enforces that background entrypoints never resolve a user's. Attribution fails open: it can never refuse or fail a call. It measures and does not bound — a turn is still not debited against AI credits
- **Experience provenance:** every banked achievement records whether the CANDIDATE asserted it (`cv_import`/`stated_in_chat`/`manual`) or the MODEL did (`agent_inferred`). Only the former may be written into a CV, and the check lives in the service path, not in a system prompt
- **Sentry:** Opt-in, env-gated, errors-only — `sentry.Init` with `SendDefaultPII:false`
- **Naming — "CV", not "résumé":** Default new surfaces to **CV**. Don't mass-rename the existing `resume`/`resumeextract` packages and columns — churn without value
