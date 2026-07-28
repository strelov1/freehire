# AGENTS.md

Guidance for AI agents working in this repository.

## Working principles

- **No overengineering, and no MVP shortcuts.** Hold the middle path: don't build infrastructure before there's a concrete need (note the seam for later instead), and don't ship quick-and-dirty or "for now" hacks. Build each feature correctly and idiomatically — neither gold-plated nor a placeholder.
- **MVP stage — keep the architecture fluid.** The current structure is not load-bearing legacy. When a new feature doesn't fit cleanly, prefer reshaping the affected part over bolting on an awkward special case.
- **Surgical changes.** Clean up what your change orphaned; leave pre-existing dead code alone. Prefer a library's intended API over a clever shim.
- **English only.** All code, comments, identifiers, docs, and commits.
- **Announce shipped work.** When a user-facing feature or fix lands, offer a changelog entry on the `/blog` feed, then a longer post if it warrants one (`.svx` files in `web/src/posts/`; the `write-changelog` skill drafts them). Skip for internal-only changes.

## What this is

`freehire` ([freehire.me](https://freehire.me)) is an open-source IT job aggregator. Many source parsers feed a pipeline that normalizes jobs into one schema, deduplicates them, and enriches them with AI; served over an HTTP API with rich filters, consumed by a SvelteKit app under `web/`.

Stack: **Go + Fiber v2**, **PostgreSQL**, **sqlc**, **Meilisearch**, **Docker Compose**, **langchaingo**.

## Layout

`internal/<domain>/` — domain packages, the substantial ones carry their own AGENTS.md (see the table below).
`cmd/<name>/` — every binary is a **run-once-and-exit** worker except `cmd/server`. They are cron-driven, not daemons; they need `DATABASE_URL` and exit non-zero on failure.

Non-obvious:

- `migrations/` — the source for **both** sqlc codegen and Postgres initdb. Never edit an applied migration; add a new file.
- `sources/` — YAML board files, not Go. One file per ATS provider, plus `custom.yml` and `telegram.yml`.
- `design-system/` — a separate pnpm package, sibling to `web/`, linked via `file:../design-system`.
- `internal/db/` — **generated**; edit `internal/db/queries/*.sql` and run `make sqlc`.
- `services/pii-filter` — a standalone service, not a Go package.

## Commands

```bash
make up / make down / make logs       # start / stop / tail app + postgres in Docker
make run / make psql / make sqlc      # run server on host / psql into DB / regenerate internal/db
make reindex                          # rebuild the Meilisearch jobs index from Postgres
go build ./...  &&  go vet ./...
go test ./...                             # unit tests (no external deps)
go test -tags=integration ./internal/db/  # queue integration tests (needs Docker; testcontainers)
```

Worker gotchas (`go run ./cmd/<name>`, all need `DATABASE_URL`; run `ls cmd/` for the full list):

- `migrate` — run **before** deploying code that reads new schema. `-baseline` records on-disk files without executing; a pre-runner database auto-baselines on first run.
- `ingest` — takes one board file: `go run ./cmd/ingest sources/<provider>.yml` (or `SOURCES_FILE`).
- `enrich` / `tg-extract` — need `LLM_BASE_URL` / `LLM_API_KEY` / `LLM_MODEL`.
- `embed` / `rollup-facets` / `reindex-companies` — need `MEILI_URL` / `MEILI_MASTER_KEY`.
- `backfill-derive` — re-derives every deterministic column (facets, `role_fingerprint`, slugs) in one keyset pass; `BACKFILL_CONCURRENCY` tunes the pool. Follow with `make reindex` — it collapses newly-clustered reposts and unions their geography.
- `reindex-companies` and `rollup-views` hold their own flock. **Never stack `reindex-companies` with `make reindex`** — Meilisearch deadlocks.
- `prune` — the **only** hard-delete path. Dry-run by default; archives every removal to `pruned_jobs`.

## Module files

Each is self-contained and can be read independently.

| Area | Reference |
|---|---|
| **HTTP handlers** (response shapes, error rendering, routes) | [internal/handler/AGENTS.md](internal/handler/AGENTS.md) |
| **SQL layer** (sqlc, queries, migrations) | [internal/db/AGENTS.md](internal/db/AGENTS.md) |
| **Search** (Meili index topology, rebuild swap, reindex hazards) | [internal/search/AGENTS.md](internal/search/AGENTS.md) |
| **Accounts** (identity resolution, seizure rule, mailed codes) | [internal/accounts/AGENTS.md](internal/accounts/AGENTS.md) |
| **Auth primitives** (JWT, API keys, cookie transport, middleware) | [internal/auth/AGENTS.md](internal/auth/AGENTS.md) |
| **OAuth sign-in** (provider registry, state cookie, identity resolution) | [internal/auth/oauth/AGENTS.md](internal/auth/oauth/AGENTS.md) |
| **Per-user job tracking** (view/apply/save/track, stages, /me/tracking) | [internal/userjob/AGENTS.md](internal/userjob/AGENTS.md) |
| **Job wire shape** (the single public projection of a job) | [internal/jobview/AGENTS.md](internal/jobview/AGENTS.md) |
| **Browser tools** (relays tool frames between agent harness and extension) | [internal/browsertools/AGENTS.md](internal/browsertools/AGENTS.md) |
| **Source ingest** (board files, provider registry, validation) | [internal/sources/AGENTS.md](internal/sources/AGENTS.md) |
| **Pipeline** (Runner, dedup, UpsertJob, board health, search indexing) | [internal/pipeline/AGENTS.md](internal/pipeline/AGENTS.md) |
| **Link resolution** (outbound job URL → destination's own identity) | [internal/linksource/AGENTS.md](internal/linksource/AGENTS.md) |
| **Board contributions** (crowdsourced URL → (source, board) onboarding) | [internal/contribution/AGENTS.md](internal/contribution/AGENTS.md) |
| **Telegram** (crawl + LLM vacancy extraction) | [internal/telegram/AGENTS.md](internal/telegram/AGENTS.md) |
| **Company names** (real display names for slug-named companies) | [internal/companyname/AGENTS.md](internal/companyname/AGENTS.md) |
| **Enrichment** (Enrichment contract, LLM Provider; enums live in `internal/vocab`) | [internal/enrich/AGENTS.md](internal/enrich/AGENTS.md) |
| **Semantic embedding** (semantic_outbox, incremental embeds, reconciler) | [internal/embed/AGENTS.md](internal/embed/AGENTS.md) |
| **In-app assistant** (turn loop, tool registry, presets, transcripts) | [internal/assistant/AGENTS.md](internal/assistant/AGENTS.md) |
| **AI fit analysis** (three-stage LLM prompt-chain, score, verdict, stream) | [internal/matchanalysis/AGENTS.md](internal/matchanalysis/AGENTS.md) |
| **Experience bank** (durable employments + evidence atoms, provenance, retrieval) | [internal/experience/AGENTS.md](internal/experience/AGENTS.md) |
| **Structured CV** (LLM parse of stored CV, stamp-and-compare) | [internal/resumeextract/AGENTS.md](internal/resumeextract/AGENTS.md) |
| **CV rendering** (templates, fonts, previews) | [internal/cv/AGENTS.md](internal/cv/AGENTS.md) |
| **Geography** (country/region codes, work-mode hint, dict-only vs hybrid) | [internal/location/AGENTS.md](internal/location/AGENTS.md) |
| **Skill tagging** (alias→canonical dictionary, jobs.skills facet) | [internal/skilltag/AGENTS.md](internal/skilltag/AGENTS.md) |
| **Seniority & category** (title→seniority/category, dict-only) | [internal/classify/AGENTS.md](internal/classify/AGENTS.md) |
| **View counts** (nginx access logs → per-job views) | [internal/viewlog/AGENTS.md](internal/viewlog/AGENTS.md) |
| **YC directory** (import-yc, curated facets, matching by former names) | [internal/ycdir/AGENTS.md](internal/ycdir/AGENTS.md) |
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
- **Email ownership:** `users.email_verified`; a password registration starts unverified and is confirmed by a mailed six-digit code. An unverified, password-backed account is **seized** (password cleared, sessions revoked) when a provider-verified OAuth identity arrives for its address — the account-pre-hijacking defence
- **API keys:** Hashed at rest (SHA-256), scoped `full` or `cv`. Key management (create/list/revoke) and password change are cookie-only
- **Enrichment:** Queue-driven (`enrichment_outbox`), provider-agnostic LLM, `Sanitize` + `Validate` gate
- **Embeddings:** Queue-driven (`semantic_outbox`), incremental, reconciled by `reindex --semantic`
- **Dictionaries:** All facet dictionaries are dict-only in production — never guess, emit nothing for unknowns
- **Job deletion:** The lifecycle only soft-closes; `cmd/prune` is the sole hard-delete path
- **In-app assistant:** a bounded tool-calling loop in-process (`internal/assistant`), streamed over SSE, gated to the restricted rollout. Tools act as the authenticated caller — no credential is minted for an agent
- **Experience provenance:** every banked achievement records whether the CANDIDATE asserted it (`cv_import`/`stated_in_chat`/`manual`) or the MODEL did (`agent_inferred`). Only the former may be written into a CV, and the check lives in the service path, not in a system prompt
- **Sentry:** Opt-in, env-gated, errors-only — `sentry.Init` with `SendDefaultPII:false`
- **Naming — "CV", not "résumé":** Default new surfaces to **CV**. Don't mass-rename the existing `resume`/`resumeextract` packages and columns — churn without value
