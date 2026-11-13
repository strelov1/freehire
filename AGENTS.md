# AGENTS.md

Guidance for AI agents working in this repository.

## Working principles

- **No overengineering, and no MVP shortcuts.** Hold the middle path: don't build infrastructure before there's a concrete need (note the seam for later instead), and don't ship quick-and-dirty or "for now" hacks. Build each feature correctly and idiomatically — neither gold-plated nor a placeholder.
- **MVP stage — keep the architecture fluid.** The current structure is not load-bearing legacy. When a new feature doesn't fit cleanly, prefer reshaping the affected part over bolting on an awkward special case.
- **Surgical changes.** Clean up what your change orphaned; leave pre-existing dead code alone. Prefer a library's intended API over a clever shim.
- **English only.** All code, comments, identifiers, docs, and commits.
- **PR titles follow [Conventional Commits](https://www.conventionalcommits.org):** `type(scope)?: summary`, with type one of `feat fix perf refactor docs test ci build chore revert` and a free-form optional scope (`fix(ingest): close empty Workday boards`). Merges are squashes whose commit is the PR title, so CI (`.github/workflows/pr-title.yml`, rules in `commitlint.config.js`) fails any other title; `pnpm check:pr-title` checks one locally from stdin.
- **Read [.out-of-scope/](.out-of-scope/README.md) before proposing or starting something new.** It holds the ideas this project already declined, each with what was checked and what would reopen it. A request that matches an entry is answered with its link, not investigated again; a decision that is reversed is deleted there.

## What this is

`freehire` ([freehire.me](https://freehire.me)) is an open-source IT job aggregator. Many source parsers feed a pipeline that normalizes jobs into one schema, deduplicates them, and enriches them with AI; served over an HTTP API with rich filters, consumed by a SvelteKit app under `web/`.

Stack: **Go + Fiber v2**, **PostgreSQL**, **sqlc**, **Meilisearch**, **Docker Compose**, **langchaingo**.

## Layout

[docs/architecture.md](docs/architecture.md) is the high-level map (topology, the repository, the main flows). The rules every change meets:

`internal/<block>/<pkg>/` — every package sits in exactly one of **eleven blocks**, forming
**eight layers**. A block may import only blocks strictly below it; two blocks sharing a
layer may not see each other in either direction. Each block carries an AGENTS.md saying
what it is and what it may reach for; substantial packages carry their own (table below).

```text
8. api           handlers, middleware, realtime, OG images
7. engage        notifications, digests, onboarding, referrals
   ingest        source adapters, crawl pipeline, link import, manual intake
6. application   tracking, stages, the event ledger, mail and calendar
   search        Meili topology, the drain, saved searches, intent
5. job           the posting: identity, facets, wire shape, dedup, ghost signal
4. candidate     CV, structured extraction, experience bank, PII, matching
3. ai            enrichment, embeddings, assistant, speech, plan limits
   identity      accounts, auth, profile
2. dict          the facet dictionaries and normalisation under them
1. platform      db, worker plumbing, outbox, cache, HTTP and LLM transport
```

Enforced twice: `depguard` in `.golangci.yml` fails on the offending import line, and
`internal/platform/arch/layering` holds the same table and reports the whole graph,
including test-only imports — only because both name the build tags `integration` and
`llmlive` (`run.build-tags` in `.golangci.yml`, the `buildTags` constant in
`layering/repo_test.go`; a new tag goes in both, or the guards go blind over its files). **Adding a package means
adding it to `internal/platform/arch/layering/blocks.go`**; a package in neither fails the
guard. Two placements surprise people (both commented in the table): the LLM client
(`platform/llm`) is transport, not AI, like `safehttp`; moderator-authored vacancies and the
public submission queue are `ingest` (manual job intake, not applications).

`cmd/<name>/` — every binary is a **run-once-and-exit**, cron-driven worker that needs `DATABASE_URL` and exits non-zero on failure, except `cmd/server`, `cmd/mail-ingest` (long-lived SES inbound daemon, `Restart=always`), `cmd/facetd` (long-lived in-memory facet engine, localhost HTTP, needs no `DATABASE_URL`) and `cmd/auto-apply-orchestrate` (serves the Inngest function's HTTP callback protocol between steps, needs no `DATABASE_URL`).

Non-obvious:

- `migrations/` — the source for **both** sqlc codegen and Postgres initdb. **Never edit an applied migration; add a new file** (`pnpm check:sql` enforces it).
- `typespec/` — **the HTTP contract**, moving in one route group at a time: `make api-contract` compiles it to `openapi/api.openapi.json` and generates `internal/api/apigen` (both committed, never edited by hand). See [internal/api/AGENTS.md](internal/api/AGENTS.md#the-http-contract-typespec--openapi--ogen).
- `internal/platform/db/` — **generated**; edit `internal/platform/db/queries/*.sql` and run `make sqlc` (the hook and CI regenerate and diff).
- **Files have a size limit.** `scripts/check-file-size.mjs` (a pre-commit hook on staged files, the `file-size` CI job and `pnpm check:size` over all files) fails on a file over its `MAX_TOKENS` (tokens = bytes / 4), and its error says how to split one. Lockfiles, generated code, data assets, applied migrations and frozen history are ignored. The limit goes down step by step to 5000 as big files are split: lower it in the change that splits the files above the new value, never raise it.
- **There is no `sources/` directory.** The board catalog is the `boards` table (`internal/ingest/boardcatalog`) and the Telegram channel list is `telegram_channels`; nothing parses a board file any more. `internal/ingest/sources` is the *adapters*, not the catalog.
- `design-system/` — a separate pnpm package linked into `web/` (pnpm `link:`) and `extension/` (npm `file:`), both symlinks. **Install it before building either consumer** — neither package manager installs a linked package's own dependencies. A type it imports must be one version across all three manifests (`pnpm check:deps`; see [design-system/AGENTS.md](design-system/AGENTS.md)).
- `extension/` — the browser extension (WXT + Svelte side-panel agent client), npm-managed unlike the rest of the JS. See [extension/AGENTS.md](extension/AGENTS.md).
- `services/pii-filter` — a standalone service, not a Go package.
- **Host configuration is not in this repo.** Systemd units, nginx snippets and operator scripts (`release.sh`, `autodeploy.sh`, `gen-ingest-timers.sh`) live only in the private `freehire-ops` repository (`provision/host2/`, `scripts/host2/`; read `provision/host2/AGENTS.md`). Nothing there deploys itself: `release.sh` builds and flips the app but never touches a unit, snippet or script, so editing one is half the job — copy it to the host and `systemctl daemon-reload`; `scripts/host2/drift-check.sh` reports drift. `deploy/rollback/` stays here: those are a migration's SQL.

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

**Toolchain and worktrees: [mise](https://mise.jdx.dev).** `mise install` puts every tool
at the version CI pins (`.mise.toml`; bump both together) and installs the lefthook hooks.
Each checkout gets its own database and API port on one shared Postgres
(`scripts/dev-env.sh`: `hire_dev` and `:8080` in the primary checkout, `hire_wt_<n>` and
`:18xxx` in a worktree), so agents in two worktrees never collide. `mise run db:up` starts
that Postgres and creates and migrates this checkout's database; `db:drop` and `db:psql` do
what they say. The same environment sets `TESTDB_ADMIN_URL`, so integration tests migrate a
template there instead of starting a container per test binary — run `db:up` first, and run
them where mise's environment reaches: a shell with `eval "$(mise activate zsh)"`, or
`mise exec -- go test -tags=integration ./...`.

**Before committing** any `*.go` file: `gofmt -w` those paths (`gofmt -l .` must print
nothing), then `go vet ./...` and `go test ./...`. Do not commit if they fail. Skip this
suite when the commit has no Go.

**Formatting TS/JS/Svelte:** oxfmt, one root `.oxfmtrc.json` for `web/`, `design-system/`,
`extension/`; `pnpm run format` (npm in `extension/`) rewrites, `format:check` is the gate — see
[ci-checks](docs/agents/ci-checks.md#formatting-oxfmt).

**`go test ./...` compiles no `//go:build integration` file, and those files are not
confined to `internal/platform/db`** — there are hundreds across the module (141 in
`internal/api/handler` alone, which call unexported constructors like `newCVHandlers`). A changed
signature passes every command above except the `vet -tags=integration` line, then fails CI,
which runs `go test -tags=integration ./...`. The vet line is the cheap guard (seconds, no
Docker); run the full tagged suite when you change behaviour rather than a signature.

**Hooks:** [lefthook](https://github.com/evilmartians/lefthook) (installed by `mise install`,
or `lefthook install` once per clone) runs gofmt/vet/golangci-lint on staged Go and eslint on
staged files in web/, extension/, design-system/ — ratcheted like CI, so only new issues
fail. `gitleaks` (`brew install gitleaks`) scans every commit and fails hard if missing; a
real finding means **revoke the credential**.

**CI gates** — each is explained, with its escape hatch, in
[docs/agents/ci-checks.md](docs/agents/ci-checks.md): migration edits and squawk
(`pnpm check:sql`, `Migration-Edit:` trailer), sqlc drift, knip dead JS incl. unused exports
(`pnpm check:dead`), Go `deadcode`, actionlint + zizmor (SHA-pinned actions), agnix (32 KiB
cap per AGENTS.md), rumdl, ast-grep, Markdown links (`pnpm check:links`), oasdiff on
`web/static/openapi.yaml` (`API-Break:` trailer), Schemathesis over the TypeSpec contract, the official MCP conformance suite over OJCP's MCP, changed-line coverage (diff-cover, Go
≥ 60%), shellcheck/hadolint, shared JS deps (`pnpm check:deps`), Ruff on Python, gitleaks.

**Before you run, change or add a `cmd/<name>` worker, read its entry in [docs/agents/workers.md](docs/agents/workers.md) when it has one**: what it needs, when it runs, and the trap behind it. Not every worker has an entry yet (`ls cmd/` is the full list); without one, read the worker's own `main.go`. A new worker gets its entry there.

## Module files

Each is self-contained and can be read independently.

| Area | Reference |
|---|---|
| **System architecture** — start here | [docs/architecture.md](docs/architecture.md) |
| **CI gates and linters** | [docs/agents/ci-checks.md](docs/agents/ci-checks.md) |
| **Public figures** (catalogue/source stats, logos, sign-up attribution) | [docs/agents/public-figures.md](docs/agents/public-figures.md) |
| **Browser extension** | [extension/AGENTS.md](extension/AGENTS.md) |
| **Company identity** (slug rule, aliases, merge) | [docs/agents/company-identity.md](docs/agents/company-identity.md) |
| **Mail stack** (inbound → classify → link → stage) | [docs/agents/mail-stack.md](docs/agents/mail-stack.md) |
| **Notifications** (digests, reminders, channels) | [docs/agents/notifications.md](docs/agents/notifications.md) |
| **Job lifecycle** (soft-close, sweep, liveness) | [docs/agents/job-lifecycle.md](docs/agents/job-lifecycle.md) |
| **Worker commands** (each `cmd/<name>`) | [docs/agents/workers.md](docs/agents/workers.md) |
| **Company facets** (remote_regions, yc_*) | [docs/agents/company-facets.md](docs/agents/company-facets.md) |
| **SPA** (SvelteKit, auth flow, API conventions) | [web/AGENTS.md](web/AGENTS.md) |
| **Host configuration** — NOT in this repo | the private `freehire-ops`: `provision/host2/AGENTS.md` |
| **Design system** (tokens, Tailwind @source, shared deps) | [design-system/AGENTS.md](design-system/AGENTS.md) |
| **`internal/platform`** block | [internal/platform/AGENTS.md](internal/platform/AGENTS.md) |
| **SQL layer** (sqlc, queries, migrations) | [internal/platform/db/AGENTS.md](internal/platform/db/AGENTS.md) |
| **Cron worker plumbing** | [internal/platform/worker/AGENTS.md](internal/platform/worker/AGENTS.md) |
| **LLM client** | [internal/platform/llm/AGENTS.md](internal/platform/llm/AGENTS.md) |
| **Headless browser** | [internal/platform/browser/AGENTS.md](internal/platform/browser/AGENTS.md) |
| **Scrapling fetch tier** | [internal/platform/scraplingfetch/AGENTS.md](internal/platform/scraplingfetch/AGENTS.md) |
| **Exception logs and metrics** (OpenTelemetry, Prometheus) | [internal/platform/observability/AGENTS.md](internal/platform/observability/AGENTS.md) |
| **`internal/dict`** block | [internal/dict/AGENTS.md](internal/dict/AGENTS.md) |
| **Company names** | [internal/dict/companyname/AGENTS.md](internal/dict/companyname/AGENTS.md) |
| **Enrichment** (contract, provider) | [internal/ai/enrich/AGENTS.md](internal/ai/enrich/AGENTS.md) |
| **Geography** | [internal/dict/location/AGENTS.md](internal/dict/location/AGENTS.md) |
| **Skill tagging** | [internal/dict/skilltag/AGENTS.md](internal/dict/skilltag/AGENTS.md) |
| **Answer topics** | [internal/dict/answertopic/AGENTS.md](internal/dict/answertopic/AGENTS.md) |
| **Skill vectors** | [internal/dict/skillvec/AGENTS.md](internal/dict/skillvec/AGENTS.md) |
| **Seniority & category** | [internal/dict/classify/AGENTS.md](internal/dict/classify/AGENTS.md) |
| **`internal/ai`** block | [internal/ai/AGENTS.md](internal/ai/AGENTS.md) |
| **Browser tools** (harness ↔ extension relay) | [internal/ai/browsertools/AGENTS.md](internal/ai/browsertools/AGENTS.md) |
| **Semantic embedding** | [internal/ai/embed/AGENTS.md](internal/ai/embed/AGENTS.md) |
| **In-app assistant** | [internal/ai/assistant/AGENTS.md](internal/ai/assistant/AGENTS.md) |
| **Speech to text** | [internal/ai/speech/AGENTS.md](internal/ai/speech/AGENTS.md) |
| **LLM spend attribution** | [internal/ai/llmkey/AGENTS.md](internal/ai/llmkey/AGENTS.md) |
| **Plan limits and the paid gate** | [internal/ai/plan/AGENTS.md](internal/ai/plan/AGENTS.md) |
| **`internal/identity`** block | [internal/identity/AGENTS.md](internal/identity/AGENTS.md) |
| **Accounts** | [internal/identity/accounts/AGENTS.md](internal/identity/accounts/AGENTS.md) |
| **Auth primitives** | [internal/identity/auth/AGENTS.md](internal/identity/auth/AGENTS.md) |
| **OAuth sign-in** | [internal/identity/auth/oauth/AGENTS.md](internal/identity/auth/oauth/AGENTS.md) |
| **Discounts** (promo codes, invites) | [internal/identity/promo/AGENTS.md](internal/identity/promo/AGENTS.md) |
| **`internal/candidate`** block | [internal/candidate/AGENTS.md](internal/candidate/AGENTS.md) |
| **AI fit analysis** (prompt chain) | [internal/candidate/matchanalysis/AGENTS.md](internal/candidate/matchanalysis/AGENTS.md) |
| **Fit-analysis use cases** (cache, credit) | [internal/candidate/fitanalysis/AGENTS.md](internal/candidate/fitanalysis/AGENTS.md) |
| **Job-match scoring** | [internal/candidate/cvmatch/AGENTS.md](internal/candidate/cvmatch/AGENTS.md) |
| **ATS-readiness score** | [internal/candidate/atscheck/AGENTS.md](internal/candidate/atscheck/AGENTS.md) |
| **Experience bank** | [internal/candidate/experience/AGENTS.md](internal/candidate/experience/AGENTS.md) |
| **Screening answer bank** | [internal/candidate/answerbank/AGENTS.md](internal/candidate/answerbank/AGENTS.md) |
| **Résumé identity** | [internal/candidate/resume/AGENTS.md](internal/candidate/resume/AGENTS.md) |
| **Talent Network** | [internal/candidate/talentnetwork/AGENTS.md](internal/candidate/talentnetwork/AGENTS.md) |
| **Structured CV** | [internal/candidate/resumeextract/AGENTS.md](internal/candidate/resumeextract/AGENTS.md) |
| **PII masking** | [internal/candidate/pii/AGENTS.md](internal/candidate/pii/AGENTS.md) |
| **Cover letters** | [internal/candidate/coverletter/AGENTS.md](internal/candidate/coverletter/AGENTS.md) |
| **CV rendering** | [internal/candidate/cv/AGENTS.md](internal/candidate/cv/AGENTS.md) |
| **CV edits** (the only writer) | [internal/candidate/cvedit/AGENTS.md](internal/candidate/cvedit/AGENTS.md) |
| **`internal/job`** block | [internal/job/AGENTS.md](internal/job/AGENTS.md) |
| **Job wire shape** | [internal/job/jobview/AGENTS.md](internal/job/jobview/AGENTS.md) |
| **Job fingerprints** (which hash for which job) | [internal/job/jobhash/AGENTS.md](internal/job/jobhash/AGENTS.md) |
| **Requirements extraction** | [internal/job/reqextract/AGENTS.md](internal/job/reqextract/AGENTS.md) |
| **Ghost detection** | [internal/job/ghost/AGENTS.md](internal/job/ghost/AGENTS.md) |
| **Search announcements** (Indexing API, IndexNow) | [internal/job/searchping/AGENTS.md](internal/job/searchping/AGENTS.md) |
| **YC directory** | [internal/job/ycdir/AGENTS.md](internal/job/ycdir/AGENTS.md) |
| **Wikidata company matching** | [internal/job/wikicompany/AGENTS.md](internal/job/wikicompany/AGENTS.md) |
| **Company collections** | [internal/job/collections/AGENTS.md](internal/job/collections/AGENTS.md) |
| **`internal/application`** block | [internal/application/AGENTS.md](internal/application/AGENTS.md) |
| **Per-user job tracking** | [internal/application/userjob/AGENTS.md](internal/application/userjob/AGENTS.md) |
| **View counts** | [internal/application/viewlog/AGENTS.md](internal/application/viewlog/AGENTS.md) |
| **Agent views** | [internal/application/agentview/AGENTS.md](internal/application/agentview/AGENTS.md) |
| **Auto-apply queue** | [internal/application/autoapply/AGENTS.md](internal/application/autoapply/AGENTS.md) |
| **`internal/search`** block; **Search** (index topology, reindex, dropped filters) | [internal/search/AGENTS.md](internal/search/AGENTS.md) |
| **Facet-search drain** | [internal/search/searchdrain/AGENTS.md](internal/search/searchdrain/AGENTS.md) |
| **Facet engine** (facetd's in-memory index, filter evaluator, the Meilisearch rules it copies) | [internal/search/facetidx/AGENTS.md](internal/search/facetidx/AGENTS.md) |
| **`internal/ingest`** block | [internal/ingest/AGENTS.md](internal/ingest/AGENTS.md) |
| **Source ingest** (adapters, registry) | [internal/ingest/sources/AGENTS.md](internal/ingest/sources/AGENTS.md) |
| **Ingest scheduling** | [internal/ingest/ingestsched/AGENTS.md](internal/ingest/ingestsched/AGENTS.md) |
| **Board catalog** (`boards`, `cmd/add-board`) | [internal/ingest/boardcatalog/AGENTS.md](internal/ingest/boardcatalog/AGENTS.md) |
| **Pipeline** (Runner, dedup, board health) | [internal/ingest/pipeline/AGENTS.md](internal/ingest/pipeline/AGENTS.md) |
| **Apply-form capture** | [internal/ingest/applyform/AGENTS.md](internal/ingest/applyform/AGENTS.md) |
| **Screening answers** | [internal/ingest/screeninganswers/AGENTS.md](internal/ingest/screeninganswers/AGENTS.md) |
| **Link resolution** | [internal/ingest/linksource/AGENTS.md](internal/ingest/linksource/AGENTS.md) |
| **ATS board recognition** | [internal/ingest/atsboard/AGENTS.md](internal/ingest/atsboard/AGENTS.md) |
| **Board contributions** | [internal/ingest/contribution/AGENTS.md](internal/ingest/contribution/AGENTS.md) |
| **Employer accounts** | [internal/ingest/employer/AGENTS.md](internal/ingest/employer/AGENTS.md) |
| **On-site applications** | [internal/ingest/directapply/AGENTS.md](internal/ingest/directapply/AGENTS.md) |
| **Telegram** | [internal/ingest/telegram/AGENTS.md](internal/ingest/telegram/AGENTS.md) |
| **`internal/engage`** block | [internal/engage/AGENTS.md](internal/engage/AGENTS.md) |
| **CV lifecycle letters** | [internal/engage/cvlifecycle/AGENTS.md](internal/engage/cvlifecycle/AGENTS.md) |
| **Interview buddies** | [internal/engage/buddy/AGENTS.md](internal/engage/buddy/AGENTS.md) |
| **Messaging core** | [internal/engage/messaging/AGENTS.md](internal/engage/messaging/AGENTS.md) |
| **Employee referrals** | [internal/engage/referral/AGENTS.md](internal/engage/referral/AGENTS.md) |
| **Mentorship marketplace** | [internal/engage/mentorship/AGENTS.md](internal/engage/mentorship/AGENTS.md) |
| **Daily social digest** | [internal/engage/socialdigest/AGENTS.md](internal/engage/socialdigest/AGENTS.md) |
| **LinkedIn credential** | [internal/engage/linkedinauth/AGENTS.md](internal/engage/linkedinauth/AGENTS.md) |
| **Discord paid channels** | [internal/engage/discordlink/AGENTS.md](internal/engage/discordlink/AGENTS.md) |
| **`internal/api`** block | [internal/api/AGENTS.md](internal/api/AGENTS.md) |
| **HTTP handlers** (responses, errors, routes) | [internal/api/handler/AGENTS.md](internal/api/handler/AGENTS.md) |
| **Auto-apply browser driver** | [internal/api/atsapply/AGENTS.md](internal/api/atsapply/AGENTS.md) |
| **ChatGPT app** (the second MCP server) | [internal/api/mcpapp/AGENTS.md](internal/api/mcpapp/AGENTS.md) |
| **OJCP provider** | [internal/api/ojcp/AGENTS.md](internal/api/ojcp/AGENTS.md) |

## Conventions

- **Response shapes:** Lists: `{"data": ..., "meta": {...}}`; single items: `{"data": ...}`; errors: `{"error": msg}`
- **A dropped filter says so:** an endpoint whose answer WIDENS on a query param it does not understand lists it in `meta.ignored_params`; an unknown closed-vocabulary value goes in `meta.ignored_values`; a subscription to a search that does not narrow (`search.Narrows`) is refused. Details in [internal/search/AGENTS.md](internal/search/AGENTS.md#a-dropped-filter-says-so-metaignored_params-metaignored_values)
- **Dedup key:** `jobs.UNIQUE (source, external_id)` — `UpsertJob` is `ON CONFLICT` on it
- **Company key:** `normalize.CompanySlug`, never `normalize.Slug` — the corporate form is not part of who the employer is. One legal-form vocabulary exists (`internal/dict/normalize`) and a test walks the module to keep it that way. Spellings a rule cannot collapse (`dollartree` vs `dollar-tree`) resolve through `company_slug_aliases`, the one company-adjacent table NOT derived from `jobs`
- **Auth:** JWT in httpOnly cookie, same-origin, carrying the account's `token_version` so sessions are revocable. `RequireAuth` (cookie only) / `RequireAuthOrKey` (cookie or full-scope Bearer) / `RequireAuthOrScopedKey` (also admits a narrow key)
- **Email ownership:** `users.email_verified`; a password registration starts unverified and is confirmed by a mailed six-digit code. An unverified, password-backed account is **seized** (password cleared, sessions revoked, API keys deleted) when a provider-verified OAuth identity arrives for its address — the account-pre-hijacking defence
- **API keys:** Hashed at rest (SHA-256), scoped `full` or `cv`, mintable only by an account with a verified address. Key management and password change are cookie-only. A key does not carry the session generation, so a `token_version` bump does not revoke it — intentional for sign-out-everywhere, wrong for a takeover, so the seizure and the mailed-code password reset delete the rows in the same statement
- **Enrichment:** Queue-driven (`enrichment_outbox`), provider-agnostic LLM, `Sanitize` + `Validate` gate
- **Embeddings:** Queue-driven (`semantic_outbox`), incremental (`cmd/embed`) — pgvector-backed `job_semantic_chunks` plus a legacy single-vector column, no search index. Reconciled by bumping the embedder-model version string, which re-enqueues the catalogue through the staleness check
- **Public figures:** catalogue size and per-source counts read ONE snapshot from `cmd/rollup-stats` (`internal/ingest/catalogstats`, `internal/ingest/sourcestats`) — never count on a request path; pass `exact` through so a degraded zero never renders as a measurement; `browsable_jobs` is NULL when unmeasured, never 0; ATS-unmatched postings are never called "exclusive". Source logos resolve by display name with `SOURCE_LOGO_DOMAINS` as override, rendered through `EntityLogo`. Sign-up attribution (`POST /api/v1/me/attribution`, server-side GA4 `sign_up`) never sends a client id the visitor did not give. All in [docs/agents/public-figures.md](docs/agents/public-figures.md)
- **Dictionaries:** All facet dictionaries are dict-only in production — never guess, emit nothing for unknowns
- **View counts:** "how much a posting was LOOKED AT" reads `job_daily_views.page_uniques`, **never `uniques`** (which includes unfiltered API reads, mostly crawlers); `jobs.view_count` is total reach, people + API + agents. Tests seed the two columns to DIFFERENT totals. See [internal/application/viewlog/AGENTS.md](internal/application/viewlog/AGENTS.md)
- **Job deletion:** The lifecycle only soft-closes; nothing hard-deletes jobs. Rows the removed `cmd/prune` took are archived in `pruned_jobs`, and the applications it orphaned carry a NULL `job_id`, which the tracking code handles
- **In-app assistant:** a bounded tool-calling loop in-process (`internal/ai/assistant`), streamed over SSE, open to every signed-in user. Tools act as the authenticated caller — no credential is minted for an agent
- **No free AI — the paid gate:** every model call made for a user's action needs a live Pro or Ultra entitlement, except FOUR `plan.WithFreeAI` marks every account gets: parsing a stored CV, the onboarding CV read (`onboarding_cv.go`, the only one open to a guest, bounded per IP at the gateway), the search interpretation (`search_intent.go`) and the support agent (self-metered, fails closed). A product invariant, not a quota: no free allowance, no override. Enforced by `mw.paidAI(feature)` (402 unpaid, 503 unreadable, before any stream) and by the handler's `llmBinding`, which refuses a model client to a context not marked `plan.WithPaidAI`/`WithFreeAI` — so mount every new model-calling route behind the middleware. Cached AI results, deterministic scoring, search and the public CV roast stay free; ownerless background work is unaffected. See [internal/ai/plan/AGENTS.md](internal/ai/plan/AGENTS.md)
- **Plan limits:** every metered AI action also draws on a per-feature, per-UTC-day allowance (`internal/ai/plan`); no currency, no balance. Tailoring has two bounds (sessions per day, turns per session). Every metered feature ships ENFORCING; `PLAN_ENFORCE` turns a FUTURE feature on after its shadow run. Refusal is HTTP 402, and on a stream it precedes the stream. Three tiers (free, pro, ultra) resolved by `TierOf` as the better live one (`users.pro_until`, `users.ultra_until`, each derived from three source columns); `PLAN_PRO_DAILY_<F>` / `PLAN_ULTRA_DAILY_<F>` move a figure without a deploy; Ultra is offered only when `STRIPE_ULTRA_PRICE_IDS` is set
- **LLM spend attribution:** every model call made for a signed-in user goes out on that user's OWN gateway credential (`internal/ai/llmkey` — minted lazily, never shown to them) and carries a `feature:` tag. Work that belongs to nobody — enrichment, Telegram, embeddings — keeps the service credential, and a test enforces that background entrypoints never resolve a user's. Attribution fails open: it never refuses or fails a call. It measures and does not bound — the plan bounds
- **Experience provenance:** every banked achievement records whether the CANDIDATE asserted it (`cv_import`/`stated_in_chat`/`manual`) or the MODEL did (`agent_inferred`). Only the former may be written into a CV, and the check lives in the service path, not in a system prompt
- **Error reporting:** OpenTelemetry exception logs (OTLP → Vector → VictoriaLogs), opt-in and env-gated; only genuine faults are reported (see `handler.classify`). There is no Sentry
- **Naming — "CV", not "résumé":** Default new surfaces to **CV**. Don't mass-rename the existing `resume`/`resumeextract` packages and columns — churn without value
