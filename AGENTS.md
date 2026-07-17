# AGENTS.md

Guidance for AI agents working in this repository.

## Working principles

Non-negotiable. Bias toward caution over speed; use judgment on trivial tasks.

- **Think before coding.** Surface assumptions. If multiple interpretations exist, present them — don't pick silently. If something is unclear, ask.
- **Simplicity first.** Minimum code that solves the problem. No features, abstractions, or error handling that wasn't asked for. Prefer a library's intended API over a clever shim.
- **Surgical changes.** Touch only what the task requires; don't refactor unbroken things or rework formatting. Match existing style. Clean up what your change orphaned; leave pre-existing dead code alone. Exception: do the real refactor when a clean change genuinely requires reshaping existing code.
- **Fix root causes, not symptoms.**
- **No overengineering, and no MVP shortcuts.** Hold the middle path: don't build infrastructure before there's a concrete need (note the seam for later instead), and don't ship quick-and-dirty or "for now" hacks. Build each feature correctly and idiomatically — neither gold-plated nor a placeholder.
- **MVP stage — keep the architecture fluid.** The project is early/MVP; the current structure is not load-bearing legacy. When a new feature doesn't fit the existing architecture cleanly, prefer reshaping or refactoring the affected part over bolting on an awkward special case — re-architect freely to keep the design clean rather than accumulating legacy.
- **English only.** All code, comments, identifiers, docs, and commits are in English.
- **Announce shipped work.** When a user-facing feature or fix lands, close the loop by offering a changelog entry on the `/blog` feed, then a longer blog post if it warrants one (posts are `.svx` files in `web/src/posts/`; the `write-changelog` skill drafts them). Skip for internal-only changes.

## What this is

`freehire` ([freehire.dev](https://freehire.dev)) is an open-source IT job aggregator backend. Intended shape: many source parsers feed a pipeline that normalizes jobs into one schema, deduplicates them, and enriches them with AI; served over an HTTP API with rich filters.

**Current state: working backend.** Fiber HTTP server with `/health`, `/api/v1/jobs[/:slug]`, Meilisearch-backed `/api/v1/jobs/search`, companies endpoints, a `/api/v1/auth` surface (register/login/me with stateless JWT + OAuth sign-in), per-user job-interaction endpoints (view/apply/save/track, behind auth, addressed by slug), and API-key management under `/api/v1/me`; Postgres via sqlc with `jobs`, `companies`, `users`, `user_jobs`, `user_identities`, and `api_keys` tables; a typed, versioned enrichment schema on `jobs`; and a family of standalone, run-once-and-exit workers: `cmd/ingest` (crawls one board file, normalizes and upserts, enqueues new postings for enrichment), `cmd/enrich` (drains the enrichment outbox via an LLM), `cmd/embed` (incremental semantic-embedding worker), `cmd/tg-ingest`/`cmd/tg-extract` (Telegram crawl + LLM-extract), `cmd/liveness` (URL-probes orphan jobs, closes dead ones), `cmd/reindex`/`cmd/backfill-derive`/`cmd/reslug` (maintenance), `cmd/rollup-stats` (activity rollup), `cmd/rollup-facets` (daily /open facet snapshot), `cmd/import-yc` (YC directory enrichment). A Svelte SPA lives under `web/` and consumes the API.

Stack: **Go + Fiber v2**, **PostgreSQL**, **sqlc**, **Docker Compose**, **langchaingo**.

## Layout

```
cmd/server/main.go        entry point: Fiber startup + graceful shutdown
cmd/ingest/main.go         source-ingest worker (crawls one board file per run)
cmd/enrich/main.go         AI enrichment worker (drains outbox queue)
cmd/embed/main.go          incremental semantic-embedding worker
cmd/tg-ingest/main.go      Telegram crawl worker
cmd/tg-extract/main.go     LLM-extracts Telegram vacancies
cmd/liveness/main.go       URL-probes orphan jobs, closes dead ones
cmd/reindex/main.go        rebuilds the Meilisearch jobs index
cmd/rollup-stats/main.go   recomputes the job_daily_stats rollup
cmd/rollup-facets/main.go  recomputes the insights_facet_stats snapshot (/open facets)
cmd/backfill-derive/main.go  re-derives all deterministic dictionary facets
cmd/reslug/main.go         backfills public_slug/company_slug
cmd/import-yc/main.go      enriches companies from yc-oss directory
sources/                   board files + sources/custom.yml + sources/telegram.yml
internal/
  config/            env config (server: PORT, DATABASE_URL, FRONTEND_ORIGIN, JWT_SECRET/JWT_TTL, COOKIE_SECURE, MEILI_URL/MEILI_MASTER_KEY, OAUTH_*, SENTRY_*; workers: LLM_BASE_URL/LLM_API_KEY/LLM_MODEL, EMBED_*)
  observability/     optional Sentry error reporting (see observability/AGENTS.md)
  database/          pgxpool connection pool
  db/                GENERATED sqlc code + queries/*.sql (see db/AGENTS.md)
  handler/           HTTP handlers + route wiring (see handler/AGENTS.md)
  auth/              auth primitives: bcrypt, JWT Issuer, API-key hashing, cookie transport (see auth/AGENTS.md)
  auth/oauth/        OAuth sign-in: Provider interface, registry, CSRF state cookie (see auth/oauth/AGENTS.md)
  sources/           ATS source adapters + registry + HTTP client + board-file parsing (see sources/AGENTS.md)
  linksource/        resolves outbound job-detail URLs (see linksource/AGENTS.md)
  telegram/          Telegram crawl + LLM vacancy extraction (see telegram/AGENTS.md)
  pipeline/          ingest Runner (fetch → normalize → dedup → upsert) (see pipeline/AGENTS.md)
  enrich/            enrichment contract + LLM Provider + queue-draining Runner (see enrich/AGENTS.md)
  embed/             incremental semantic embedding (see embed/AGENTS.md)
  search/            Meilisearch-backed job search
  location/          curated dictionary deriving country/region codes + work-mode hint (see location/AGENTS.md)
  ycdir/             yc-oss directory to company-info mapping (see ycdir/AGENTS.md)
  job/               Job domain aggregate: sealed type built only through job.New
  jobview/           single public wire shape of a job, projected from Job aggregate
  normalize/         slug normalization
  jobfit/            AI fit analysis: three-stage LLM prompt-chain (see jobfit/AGENTS.md)
  resumeextract/     structured résumé extraction from stored CV (see resumeextract/AGENTS.md)
  userjob/           per-user job tracking (see userjob/AGENTS.md)
  classify/          seniority/category tagging from job title (see classify/AGENTS.md)
  skilltag/          deterministic skill tagging dictionary (see skilltag/AGENTS.md)
migrations/          SQL schema — source for BOTH sqlc and Postgres initdb
```

## Commands

```bash
make up / make down / make logs       # start / stop / tail app + postgres in Docker
make run / make psql / make sqlc      # run server on host / psql into DB / regenerate internal/db
make reindex                          # rebuild the Meilisearch index from Postgres
go build ./...  &&  go vet ./...
go test ./...                              # unit tests (no external deps)
go test -tags=integration ./internal/db/  # queue integration tests (needs Docker; testcontainers)
# run-once-and-exit cron workers (all need DATABASE_URL):
go run ./cmd/ingest sources/<provider>.yml # crawl one board file (path as arg or SOURCES_FILE)
go run ./cmd/enrich                        # + LLM_BASE_URL/LLM_API_KEY/LLM_MODEL
go run ./cmd/embed                         # + MEILI_URL/MEILI_MASTER_KEY (+ EMBED_* to tune)
go run ./cmd/tg-ingest                     # crawl sources/telegram.yml (path via CHANNELS_FILE)
go run ./cmd/tg-extract                    # + LLM_* — drain telegram_posts into the catalogue
go run ./cmd/liveness                      # URL-probe orphan jobs, close dead ones
go run ./cmd/backfill-derive               # re-derive dictionary facets; follow with make reindex
go run ./cmd/rollup-stats                  # recompute job_daily_stats (run-once, cron ~every 3h)
go run ./cmd/rollup-facets                 # + MEILI_URL/MEILI_MASTER_KEY — recompute insights_facet_stats (run-once, cron ~daily)
```

For the full architecture and conventions, see the **module files** below. Each module is self-contained and can be read independently.

## Module files

### Backend core

| Area | Reference |
|---|---|
| **HTTP handlers** (response shapes, error rendering, routes) | [internal/handler/AGENTS.md](internal/handler/AGENTS.md) |
| **SQL layer** (sqlc, queries, migrations) | [internal/db/AGENTS.md](internal/db/AGENTS.md) |
| **Auth primitives** (JWT, API keys, cookie transport, middleware) | [internal/auth/AGENTS.md](internal/auth/AGENTS.md) |
| **OAuth sign-in** (provider registry, state cookie, identity resolution) | [internal/auth/oauth/AGENTS.md](internal/auth/oauth/AGENTS.md) |
| **Per-user job tracking** (view/apply/save/track, stages, /me/tracking) | [internal/userjob/AGENTS.md](internal/userjob/AGENTS.md) |

### Ingest pipeline

| Area | Reference |
|---|---|
| **Source ingest** (board files, provider registry, validation, USAJobs) | [internal/sources/AGENTS.md](internal/sources/AGENTS.md) |
| **Pipeline** (Runner, dedup, UpsertJob, board health, search indexing) | [internal/pipeline/AGENTS.md](internal/pipeline/AGENTS.md) |
| **Link resolution** (outbound job URL → destination's own identity) | [internal/linksource/AGENTS.md](internal/linksource/AGENTS.md) |

### AI enrichment

| Area | Reference |
|---|---|
| **Enrichment** (Enrichment contract, controlled vocabularies, LLM Provider) | [internal/enrich/AGENTS.md](internal/enrich/AGENTS.md) |
| **Semantic embedding** (semantic_outbox, incremental embeds, reconciler) | [internal/embed/AGENTS.md](internal/embed/AGENTS.md) |
| **AI fit analysis** (three-stage LLM prompt-chain, score, verdict, stream) | [internal/jobfit/AGENTS.md](internal/jobfit/AGENTS.md) |
| **Structured résumé** (LLM parse of stored CV, stamp-and-compare) | [internal/resumeextract/AGENTS.md](internal/resumeextract/AGENTS.md) |

### Dictionary facets

| Area | Reference |
|---|---|
| **Geography** (country/region codes, work-mode hint, dict-only vs hybrid) | [internal/location/AGENTS.md](internal/location/AGENTS.md) |
| **Skill tagging** (alias→canonical dictionary, jobs.skills facet) | [internal/skilltag/AGENTS.md](internal/skilltag/AGENTS.md) |
| **Seniority & category** (title→seniority/category, dict-only) | [internal/classify/AGENTS.md](internal/classify/AGENTS.md) |

### Cross-cutting

| Area | Reference |
|---|---|
| **Job lifecycle** (soft-close, ingest sweep, self-close, liveness probe) | [docs/agents/job-lifecycle.md](docs/agents/job-lifecycle.md) |
| **Company facets** (remote_regions vs yc_* curated facets) | [docs/agents/company-facets.md](docs/agents/company-facets.md) |
| **Sentry error tracking** (backend, workers, frontend — env-gated) | [internal/observability/AGENTS.md](internal/observability/AGENTS.md) |
| **YC directory** (import-yc, curated facets, matching by former names) | [internal/ycdir/AGENTS.md](internal/ycdir/AGENTS.md) |

### Frontend

| Area | Reference |
|---|---|
| **SPA sub-context** (SvelteKit, auth flow, API conventions) | [web/AGENTS.md](web/AGENTS.md) |

## Conventions (quick reference)

> **Full details in the module files above.** This section is a quick-reference only.

- **Response shapes:** Lists: `{"data": ..., "meta": {...}}`; single items: `{"data": ...}`; errors: `{"error": msg}`
- **Dedup key:** `jobs.UNIQUE (source, external_id)` — `UpsertJob` is `ON CONFLICT` on it
- **Auth:** Stateless JWT in httpOnly cookie, same-origin. `RequireAuth` (cookie only) / `RequireAuthOrKey` (cookie or Bearer)
- **API keys:** Hashed at rest (SHA-256). Key management (create/list/revoke) is cookie-only
- **Enrichment:** Queue-driven (`enrichment_outbox`), provider-agnostic LLM, `Sanitize` + `Validate` gate
- **Embeddings:** Queue-driven (`semantic_outbox`), incremental, reconciled by `reindex --semantic`
- **Dictionaries:** All facet dictionaries are dict-only in production (never guess, emit nothing for unknowns)
- **Migrations:** Via Postgres initdb — single-run on first volume init only; recreate volume to re-apply
- **Sentry:** Opt-in, env-gated, errors-only — `sentry.Init` with `SendDefaultPII:false`
