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

`internal/<block>/<pkg>/` — every package sits in exactly one of **eleven blocks**, and the
blocks form **eight layers**. A block may import only blocks strictly below it; two blocks
sharing a layer may not see each other in either direction. The substantial packages carry
their own AGENTS.md (see the table below), and each block carries one saying what it is and
what it may reach for.

```
8. api           handlers, middleware, realtime, OG images
7. engage        notifications, digests, onboarding, referrals
   ingest        source adapters, crawl pipeline, link import, manual intake
6. application   tracking, stages, the event ledger, mail and calendar
   search        Meili topology, the drain, saved searches, intent
5. job           the posting: identity, facets, wire shape, dedup, ghost signal
4. candidate     CV, structured extraction, experience bank, PII, matching
3. ai            enrichment, embeddings, assistant, speech, credits
   identity      accounts, auth, profile
2. dict          the facet dictionaries and normalisation under them
1. platform      db, worker plumbing, outbox, cache, HTTP and LLM transport
```

The rule is enforced twice: `depguard` in `.golangci.yml` fails on the offending import
line, and `internal/platform/arch/layering` holds the same table and reports the whole
graph at once — including imports that exist only in test files, and only because
`run.build-tags` names `integration` and `llmlive`. Without those tags both guards go blind
over 222 files. **Adding a package means adding it to the table in
`internal/platform/arch/layering/blocks.go`**; a package in neither fails the guard.

Two placements surprise people, and both are in the table's comments: the LLM client
(`platform/llm`) is transport, not AI — it knows nothing about the domain, like `safehttp`;
and moderator-authored vacancies plus the public submission queue are `ingest`, because
they are manual job intake rather than applications.

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
- `internal/platform/db/` — **generated**; edit `internal/platform/db/queries/*.sql` and run `make sqlc`.
- `services/pii-filter` — a standalone service, not a Go package.

## Commands

```bash
make up / make down / make logs       # start / stop / tail app + postgres in Docker
make run / make psql / make sqlc      # run server on host / psql into DB / regenerate internal/platform/db
make reindex                          # rebuild the Meilisearch jobs index from Postgres
go build ./...  &&  go vet ./...
go test ./...                             # unit tests (no external deps)
go vet -tags=integration ./...            # compiles the tagged tests — run before EVERY push
go test -tags=integration ./internal/platform/db/  # queue integration tests (needs Docker; testcontainers)
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
confined to `internal/platform/db`** — there are 187 of them across 20 packages, and `internal/api/handler`
holds 78, which call unexported constructors like `newCVHandlers`. A changed signature
therefore passes every command above except the `vet` line, then fails CI, which runs
`go test -tags=integration ./...` over the whole module. The vet line is the cheap guard:
seconds, no Docker. Run the full tagged suite when you change behaviour rather than a
signature.

Worker gotchas (`go run ./cmd/<name>`, all need `DATABASE_URL`; run `ls cmd/` for the full list):

- `migrate` — run **before** deploying code that reads new schema. `-baseline` records on-disk files without executing; a pre-runner database auto-baselines on first run.
- `ingest` — takes one board file: `go run ./cmd/ingest sources/<provider>.yml` (or `SOURCES_FILE`). `HYDRATION_RETRY_DAYS` widens the window (default 14 days, measured from `created_at`) during which a stored row with NO description is withheld from the seen-set so a hydrating adapter re-attempts its detail fetch. Set it only for a deliberate repair run — `HYDRATION_RETRY_DAYS=365 ingest sources/hh.yml` re-offers every body-less row of that provider, one extra detail request each (hh's detail pages are ~1 MB), and expect to repeat it: a run that hits its timeout still persists what it hydrated and the fixed rows leave the set, so successive runs shrink the backlog. An unparseable or non-positive value fails the run rather than falling back, because a typo here would look exactly like a repair that found nothing.
- `enrich` / `tg-extract` — need `LLM_BASE_URL` / `LLM_API_KEY` / `LLM_MODEL`.
- `embed` / `search-drain` / `rollup-facets` / `reindex-companies` — need `MEILI_URL` / `MEILI_MASTER_KEY`. `search-drain` drains `search_outbox` (queued by `cmd/ingest`, atomically with each write) into the live facet index in batches — run it frequently (e.g. every 1-2 min); see [internal/search/searchdrain/AGENTS.md](internal/search/searchdrain/AGENTS.md).
- `reindex` — rebuilds the Meilisearch jobs index. **`REINDEX_DEDUP=1` additionally refreshes the duplicate markers** (role clusters, aggregator suppression, fuzzy collapse) before the rebuild; without it the rebuild uses the markers the last dedup run left. Off by default since 2026-08-16: aggregator suppression alone measured ~23h against a 12h unit timeout, so the run was cancelled mid-dedup and never reached the rebuild — 3 days with zero successful reindexes. Run the dedup invocation on its own, rarer schedule. **`REINDEX_DEDUP_ONLY=1` runs just the marker passes and exits** — no Meilisearch client, no disk guard, no rebuild, so no need to pause `search-drain` for it. It exists so the markers can refresh on a tighter cadence than the full `REINDEX_DEDUP=1` invocation: the plain (no-flag) rebuild already runs every few hours on its own schedule and picks up whatever markers are current, so a fresher marker-only pass alone shrinks the window a repost sits undeduped in search. Takes precedence over `REINDEX_DEDUP` if both are set. **A rebuild also writes the match sort's skill vectors** (`internal/dict/skillvec`), which grows the index by roughly 10 GB and makes the rebuild materially slower — the cost is HNSW graph construction, so it lands here and not on the incremental `search-drain` pushes. Schedule it deliberately, and note the embedder must be in the LIVE index settings before a binary that queries it rolls out — see [internal/search/search/AGENTS.md](internal/search/search/AGENTS.md).
- `backfill-derive` — re-derives every deterministic column (facets, `role_fingerprint`, slugs) in one keyset pass; `BACKFILL_CONCURRENCY` tunes the pool. Follow with `REINDEX_DEDUP=1 make reindex` — that is what collapses newly-clustered reposts and unions their geography.
- `backfill-slug-folded` — one-off: fills `jobs.company_slug_folded` for the rows that predate the column (migration 0109). Chunked, paced, and idempotent — the chunk UPDATE is `IS DISTINCT FROM`-guarded, so a re-run writes nothing and stopping it mid-way is free. Until it completes the aggregator-suppression pass simply matches fewer rows, never wrong ones. Needs only `DATABASE_URL`.
- `backfill-duplicate-marker-owner` — one-off: seeds `jobs.duplicate_of_{aggregator,role,fuzzy}` from the single `duplicate_of` that predates them (migration 0114). **Runs BETWEEN migrations 0114 and 0115** — 0115 derives `duplicate_of` from those three, and a derivation over three empty columns would clear every marker in the catalogue. Chunked over an id RANGE, paced, and idempotent: a row that already has an owned column set no longer matches, so a re-run writes nothing, stopping mid-way is free, and re-running it after 0115 lands IS the reconcile sweep for rows written while it was walking. `BACKFILL_MARKER_CHUNK` (default 50k) widens the id span per statement — the id sequence runs far ahead of the live row count, so most chunks cover empty stretches. Provenance is not recoverable from a stored marker, so the seed goes by shape and fuzzy markers land in the role column; the first marker refresh after deploy corrects that in one cycle. Needs only `DATABASE_URL`.
- `capture-apply-form` — drains the apply-form capture queue: fetches each queued posting's application form from `greenhouse`/`ashby`/`workable`/`lever` and stores it in `apply_forms`. Needs nothing but `DATABASE_URL`; `APPLY_FORM_CONCURRENCY` (default 4) bounds how hard one run leans on a platform and `APPLY_FORM_MAX_PER_RUN` (default 5000) how much of the backlog it takes — the second matters because the first drain faces a ~185k backlog and an unbounded run would work for hours, which `Type=oneshot` turns into silently skipped timer firings. `recruitee` forms never reach this queue — its listing carries them, so ingest writes them directly.
- `hydrate-adzuna-description` — drains the Adzuna full-description capture queue (`cmd/ingest` enqueues eligible postings; see [internal/ingest/sources/AGENTS.md](internal/ingest/sources/AGENTS.md)). `ADZUNA_DESCRIPTION_MAX_PER_RUN` (default 500) is deliberately conservative — untested against Adzuna at real crawl-host volume. `seed-adzuna-description-queue` is the one-off companion that queues the pre-existing backlog; run it once, then let the cron drain handle the rest.
- `queue-metrics` — measures outbox depth, board-fleet health, and catalogue freshness and publishes them as Prometheus gauges via the node_exporter textfile collector. Needs `DATABASE_URL`, and is a **no-op that never opens the pool** without `PROM_TEXTFILE_DIR`. Read-only and lock-free by design — run it every minute; see [internal/platform/worker/AGENTS.md](internal/platform/worker/AGENTS.md) for the published names.
- `merge-companies` — collapses the company slugs that are one employer written more than one way (`ringcentral` + `ringcentral-inc`, `dollar-tree` + `dollartree`). **Reports by default; `--apply` writes.** `--min-jobs N` bounds a wave to the folded groups whose combined open jobs reach N, which is how the backlog is taken in reviewed passes (1000, then 100, then 10, then 1) rather than one 333k-row leap. Chunked and idempotent — the chunk statement selects rows still carrying the retired slug, so a re-run moves nothing and an interrupted wave resumes. **Do NOT reindex afterwards**: a facet-index push costs 90-180s regardless of batch size, so a wave pushed through `search_outbox` is tens of hours; the scheduled `freehire-reindexw` picks it up, and until it does a merged company under-counts its jobs for a few hours without anything 404ing. `REINDEX_DEDUP` stays unset. Needs only `DATABASE_URL`. See [docs/agents/company-identity.md](docs/agents/company-identity.md).
- **Nothing holds a flock** — no file locks in Go or in the systemd units; the word survives only in a few stale comments. What keeps a cron worker from stacking on itself is systemd: a `Type=oneshot` unit will not start a second instance while the first is active. That protects the TIMER path only, so a run started by hand has no lock at all. **Never stack `reindex-companies` with `make reindex`** — Meilisearch runs one serial task queue, so the second rebuild queues behind the first and looks like a hang, and a swap transiently holds ~2x the index's disk. Two workers additionally take a Postgres advisory lock (`cmd/liveness`, `cmd/ghost-crosscheck`); their keys are listed in `internal/platform/migrate`.
- `prune` — the **only** hard-delete path. Dry-run by default; archives every removal to `pruned_jobs`.

## Module files

Each is self-contained and can be read independently.

| Area | Reference |
|---|---|
| **System architecture** (the high-level map: topology, repo layout, the three main flows) — start here | [docs/architecture.md](docs/architecture.md) |
| **Browser extension** (WXT + Svelte side-panel agent client, the other end of Browser tools) | [extension/AGENTS.md](extension/AGENTS.md) |
| **Company identity** (the company slug rule, the alias registry, the merge worker) | [docs/agents/company-identity.md](docs/agents/company-identity.md) |
| **Mail stack** (Gmail + SES inbound → classify → link → stage advance) | [docs/agents/mail-stack.md](docs/agents/mail-stack.md) |
| **Notifications** (subscription digests, saved-job reminders, channels) | [docs/agents/notifications.md](docs/agents/notifications.md) |
| **Job lifecycle** (soft-close, ingest sweep, self-close, liveness probe) | [docs/agents/job-lifecycle.md](docs/agents/job-lifecycle.md) |
| **Company facets** (remote_regions vs yc_* curated facets) | [docs/agents/company-facets.md](docs/agents/company-facets.md) |
| **SPA sub-context** (SvelteKit, auth flow, API conventions) | [web/AGENTS.md](web/AGENTS.md) |
| **Design system** (pnpm package, tokens, Tailwind @source trap) | [design-system/AGENTS.md](design-system/AGENTS.md) |
| **`internal/platform`** — the block itself: what it is, what it may import | [internal/platform/AGENTS.md](internal/platform/AGENTS.md) |
| **SQL layer** (sqlc, queries, migrations) | [internal/platform/db/AGENTS.md](internal/platform/db/AGENTS.md) |
| **Cron worker plumbing** (Main/Bootstrap, exit codes, heartbeat, corruption-tolerant scans) | [internal/platform/worker/AGENTS.md](internal/platform/worker/AGENTS.md) |
| **LLM client** (provider-agnostic wrapper, schema cache, streaming, attribution tags) | [internal/platform/llm/AGENTS.md](internal/platform/llm/AGENTS.md) |
| **Sentry error tracking** (backend, workers, frontend — env-gated) | [internal/platform/observability/AGENTS.md](internal/platform/observability/AGENTS.md) |
| **`internal/dict`** — the block itself: what it is, what it may import | [internal/dict/AGENTS.md](internal/dict/AGENTS.md) |
| **Company names** (real display names for slug-named companies) | [internal/dict/companyname/AGENTS.md](internal/dict/companyname/AGENTS.md) |
| **Enrichment** (Enrichment contract, LLM Provider; enums live in `internal/dict/vocab`) | [internal/ai/enrich/AGENTS.md](internal/ai/enrich/AGENTS.md) |
| **Geography** (country/region codes, work-mode hint, dict-only vs hybrid) | [internal/dict/location/AGENTS.md](internal/dict/location/AGENTS.md) |
| **Skill tagging** (alias→canonical dictionary, jobs.skills facet) | [internal/dict/skilltag/AGENTS.md](internal/dict/skilltag/AGENTS.md) |
| **Skill vectors** (the permanent position registry, the ballast, why coverage decides the order) | [internal/dict/skillvec/AGENTS.md](internal/dict/skillvec/AGENTS.md) |
| **Seniority & category** (title→seniority/category, dict-only) | [internal/dict/classify/AGENTS.md](internal/dict/classify/AGENTS.md) |
| **`internal/ai`** — the block itself: what it is, what it may import | [internal/ai/AGENTS.md](internal/ai/AGENTS.md) |
| **Browser tools** (relays tool frames between agent harness and extension) | [internal/ai/browsertools/AGENTS.md](internal/ai/browsertools/AGENTS.md) |
| **Semantic embedding** (semantic_outbox, incremental chunked embeds into pgvector) | [internal/ai/embed/AGENTS.md](internal/ai/embed/AGENTS.md) |
| **In-app assistant** (turn loop, tool registry, presets, transcripts) | [internal/ai/assistant/AGENTS.md](internal/ai/assistant/AGENTS.md) |
| **Speech to text** (dictation into the composer, the filename rule, spend bounds) | [internal/ai/speech/AGENTS.md](internal/ai/speech/AGENTS.md) |
| **LLM spend attribution** (per-user gateway credential, feature tags, fail-open) | [internal/ai/llmkey/AGENTS.md](internal/ai/llmkey/AGENTS.md) |
| **`internal/identity`** — the block itself: what it is, what it may import | [internal/identity/AGENTS.md](internal/identity/AGENTS.md) |
| **Accounts** (identity resolution, seizure rule, mailed codes) | [internal/identity/accounts/AGENTS.md](internal/identity/accounts/AGENTS.md) |
| **Auth primitives** (JWT, API keys, cookie transport, middleware) | [internal/identity/auth/AGENTS.md](internal/identity/auth/AGENTS.md) |
| **OAuth sign-in** (provider registry, state cookie, identity resolution) | [internal/identity/auth/oauth/AGENTS.md](internal/identity/auth/oauth/AGENTS.md) |
| **`internal/candidate`** — the block itself: what it is, what it may import | [internal/candidate/AGENTS.md](internal/candidate/AGENTS.md) |
| **AI fit analysis** (three-stage LLM prompt-chain, score, verdict, stream) | [internal/candidate/matchanalysis/AGENTS.md](internal/candidate/matchanalysis/AGENTS.md) |
| **Job-match scoring** (deterministic CV-vs-vacancy score, the unverifiable rule) | [internal/candidate/cvmatch/AGENTS.md](internal/candidate/cvmatch/AGENTS.md) |
| **ATS-readiness score** (deterministic CV score, de-identified LLM review, delta) | [internal/candidate/atscheck/AGENTS.md](internal/candidate/atscheck/AGENTS.md) |
| **Experience bank** (durable employments + evidence atoms, provenance, retrieval) | [internal/candidate/experience/AGENTS.md](internal/candidate/experience/AGENTS.md) |
| **Résumé identity** (one stored CV per user, contact-block layers) | [internal/candidate/resume/AGENTS.md](internal/candidate/resume/AGENTS.md) |
| **Structured CV** (LLM parse of stored CV, stamp-and-compare) | [internal/candidate/resumeextract/AGENTS.md](internal/candidate/resumeextract/AGENTS.md) |
| **PII masking** (fail-closed CV→LLM redaction, reversible placeholders) | [internal/candidate/pii/AGENTS.md](internal/candidate/pii/AGENTS.md) |
| **CV rendering** (templates, fonts, previews) | [internal/candidate/cv/AGENTS.md](internal/candidate/cv/AGENTS.md) |
| **CV edits** (the only writer: path operations, revisions, undo, the evidence gate) | [internal/candidate/cvedit/AGENTS.md](internal/candidate/cvedit/AGENTS.md) |
| **`internal/job`** — the block itself: what it is, what it may import | [internal/job/AGENTS.md](internal/job/AGENTS.md) |
| **Job wire shape** (the single public projection of a job) | [internal/job/jobview/AGENTS.md](internal/job/jobview/AGENTS.md) |
| **Job fingerprints** (content_hash vs role_fingerprint vs RoleKey — which hash for which job) | [internal/job/jobhash/AGENTS.md](internal/job/jobhash/AGENTS.md) |
| **Ghost detection** (hedged posting-reality verdict, two evidence tiers, crosscheck) | [internal/job/ghost/AGENTS.md](internal/job/ghost/AGENTS.md) |
| **YC directory** (import-yc, curated facets, matching by former names) | [internal/job/ycdir/AGENTS.md](internal/job/ycdir/AGENTS.md) |
| **Company collections** (curated company tags, register datasets, reconciliation) | [internal/job/collections/AGENTS.md](internal/job/collections/AGENTS.md) |
| **`internal/application`** — the block itself: what it is, what it may import | [internal/application/AGENTS.md](internal/application/AGENTS.md) |
| **Per-user job tracking** (view/apply/save/track, stages, /me/tracking) | [internal/application/userjob/AGENTS.md](internal/application/userjob/AGENTS.md) |
| **View counts** (nginx access logs → per-job views) | [internal/application/viewlog/AGENTS.md](internal/application/viewlog/AGENTS.md) |
| **`internal/search`** — the block itself: what it is, what it may import | [internal/search/AGENTS.md](internal/search/AGENTS.md) |
| **Search** (Meili index topology, rebuild swap, reindex hazards) | [internal/search/AGENTS.md](internal/search/AGENTS.md) |
| **Similar-jobs backfill** (precomputes `/similar`'s data via pgvector, no outbox) | [internal/search/similarjobs/AGENTS.md](internal/search/similarjobs/AGENTS.md) |
| **Facet-search drain** (search_outbox, incremental facet-index pushes, reconciler) | [internal/search/searchdrain/AGENTS.md](internal/search/searchdrain/AGENTS.md) |
| **`internal/ingest`** — the block itself: what it is, what it may import | [internal/ingest/AGENTS.md](internal/ingest/AGENTS.md) |
| **Source ingest** (board files, provider registry, validation) | [internal/ingest/sources/AGENTS.md](internal/ingest/sources/AGENTS.md) |
| **Pipeline** (Runner, dedup, UpsertJob, board health, search indexing) | [internal/ingest/pipeline/AGENTS.md](internal/ingest/pipeline/AGENTS.md) |
| **Apply-form capture** (ATS application forms, verbatim platform vocabulary, queue drain) | [internal/ingest/applyform/AGENTS.md](internal/ingest/applyform/AGENTS.md) |
| **Link resolution** (outbound job URL → destination's own identity) | [internal/ingest/linksource/AGENTS.md](internal/ingest/linksource/AGENTS.md) |
| **ATS board recognition** (URL → (source, board), shared conventions) | [internal/ingest/atsboard/AGENTS.md](internal/ingest/atsboard/AGENTS.md) |
| **Board contributions** (crowdsourced URL → (source, board) onboarding) | [internal/ingest/contribution/AGENTS.md](internal/ingest/contribution/AGENTS.md) |
| **Telegram** (crawl + LLM vacancy extraction) | [internal/ingest/telegram/AGENTS.md](internal/ingest/telegram/AGENTS.md) |
| **`internal/engage`** — the block itself: what it is, what it may import | [internal/engage/AGENTS.md](internal/engage/AGENTS.md) |
| **Employee referrals** (offer/request marketplace, moderation, anonymity) | [internal/engage/referral/AGENTS.md](internal/engage/referral/AGENTS.md) |
| **`internal/api`** — the block itself: what it is, what it may import | [internal/api/AGENTS.md](internal/api/AGENTS.md) |
| **HTTP handlers** (response shapes, error rendering, routes) | [internal/api/handler/AGENTS.md](internal/api/handler/AGENTS.md) |

## Conventions

- **Response shapes:** Lists: `{"data": ..., "meta": {...}}`; single items: `{"data": ...}`; errors: `{"error": msg}`
- **Dedup key:** `jobs.UNIQUE (source, external_id)` — `UpsertJob` is `ON CONFLICT` on it
- **Company key:** `normalize.CompanySlug`, never `normalize.Slug` — the corporate form is not part of who the employer is. One legal-form vocabulary exists (`internal/dict/normalize`) and a test walks the module to keep it that way; there were four, they disagreed, and every resulting miss was silent. Spellings a rule cannot collapse (`dollartree` vs `dollar-tree`) resolve through `company_slug_aliases`, the one company-adjacent table that is NOT derived from `jobs`
- **Auth:** JWT in httpOnly cookie, same-origin, carrying the account's `token_version` so sessions are revocable. `RequireAuth` (cookie only) / `RequireAuthOrKey` (cookie or full-scope Bearer) / `RequireAuthOrScopedKey` (also admits a narrow key)
- **Email ownership:** `users.email_verified`; a password registration starts unverified and is confirmed by a mailed six-digit code. An unverified, password-backed account is **seized** (password cleared, sessions revoked, API keys deleted) when a provider-verified OAuth identity arrives for its address — the account-pre-hijacking defence
- **API keys:** Hashed at rest (SHA-256), scoped `full` or `cv`, and mintable only by an account with a verified address. Key management (create/list/revoke) and password change are cookie-only. A key does not carry the session generation, so a `token_version` bump does not revoke it — that is intentional for sign-out-everywhere and wrong for a takeover, so the seizure and the mailed-code password reset delete the rows in the same statement
- **Enrichment:** Queue-driven (`enrichment_outbox`), provider-agnostic LLM, `Sanitize` + `Validate` gate
- **Embeddings:** Queue-driven (`semantic_outbox`), incremental (`cmd/embed`) — pgvector-backed `job_semantic_chunks` plus a legacy single-vector column, no search index. Reconciled by bumping the embedder-model version string, which re-enqueues the whole catalogue through the existing staleness check
- **Catalogue scale:** Every public figure describing how big the catalogue is — `GET /api/v1/stats/catalog`, the jobs list's `meta.total`, the `/about` and `/open` strips — reads ONE snapshot published by `cmd/rollup-stats` (`internal/ingest/catalogstats`). Never count on a request path: `catalogstats.Load` takes no exact counter, so it cannot. A read never fails; a cold cache, an unreachable Redis or no cache at all degrades to the planner estimate with `exact: false`, which zeroes the figures that exist only in the database — pass `exact` through to whatever renders them, because a zero must not reach a page as if it were a measurement
- **Dictionaries:** All facet dictionaries are dict-only in production — never guess, emit nothing for unknowns
- **Job deletion:** The lifecycle only soft-closes; `cmd/prune` is the sole hard-delete path
- **In-app assistant:** a bounded tool-calling loop in-process (`internal/ai/assistant`), streamed over SSE, open to every signed-in user. Tools act as the authenticated caller — no credential is minted for an agent
- **LLM spend attribution:** every model call made for a signed-in user goes out on that user's OWN gateway credential (`internal/ai/llmkey` — minted lazily on first use, never shown to them) and carries a `feature:` tag. Work that belongs to nobody — enrichment, Telegram, embeddings — keeps the service credential, and a test enforces that background entrypoints never resolve a user's. Attribution fails open: it can never refuse or fail a call. It measures and does not bound — a turn is still not debited against AI credits
- **Experience provenance:** every banked achievement records whether the CANDIDATE asserted it (`cv_import`/`stated_in_chat`/`manual`) or the MODEL did (`agent_inferred`). Only the former may be written into a CV, and the check lives in the service path, not in a system prompt
- **Sentry:** Opt-in, env-gated, errors-only — `sentry.Init` with `SendDefaultPII:false`
- **Naming — "CV", not "résumé":** Default new surfaces to **CV**. Don't mass-rename the existing `resume`/`resumeextract` packages and columns — churn without value
