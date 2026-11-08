<!-- agnix-disable CDX-AG-009 -->
# AGENTS.md

Guidance for AI agents working in this repository.

## Working principles

- **No overengineering, and no MVP shortcuts.** Hold the middle path: don't build infrastructure before there's a concrete need (note the seam for later instead), and don't ship quick-and-dirty or "for now" hacks. Build each feature correctly and idiomatically — neither gold-plated nor a placeholder.
- **MVP stage — keep the architecture fluid.** The current structure is not load-bearing legacy. When a new feature doesn't fit cleanly, prefer reshaping the affected part over bolting on an awkward special case.
- **Surgical changes.** Clean up what your change orphaned; leave pre-existing dead code alone. Prefer a library's intended API over a clever shim.
- **English only.** All code, comments, identifiers, docs, and commits.
- **Read [.out-of-scope/](.out-of-scope/README.md) before proposing or starting something new.** It holds the ideas this project already declined, each with what was checked and what would reopen it. A request that matches an entry is answered with its link, not investigated again; a decision that is reversed is deleted there.

## What this is

`freehire` ([freehire.me](https://freehire.me)) is an open-source IT job aggregator. Many source parsers feed a pipeline that normalizes jobs into one schema, deduplicates them, and enriches them with AI; served over an HTTP API with rich filters, consumed by a SvelteKit app under `web/`.

Stack: **Go + Fiber v2**, **PostgreSQL**, **sqlc**, **Meilisearch**, **Docker Compose**, **langchaingo**.

## Layout

`internal/<block>/<pkg>/` — every package sits in exactly one of **eleven blocks**, and the
blocks form **eight layers**. A block may import only blocks strictly below it; two blocks
sharing a layer may not see each other in either direction. The substantial packages carry
their own AGENTS.md (see the table below), and each block carries one saying what it is and
what it may reach for.

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

`cmd/<name>/` — every binary is a **run-once-and-exit** worker except `cmd/server`, `cmd/mail-ingest` (a long-lived SES inbound daemon, `Restart=always`), and `cmd/auto-apply-orchestrate` (serves the Inngest function's own HTTP callback protocol between steps, needs no `DATABASE_URL`). The rest are cron-driven, not daemons; they need `DATABASE_URL` and exit non-zero on failure.

Non-obvious:

- `migrations/` — the source for **both** sqlc codegen and Postgres initdb. Never edit an applied migration; add a new file. `pnpm check:sql` now ENFORCES that: it fails when a change modifies, deletes or renames a migration main already holds, unless a commit in the change carries a `Migration-Edit: <reason>` trailer — for the rare edit that is right (a comment, a file that never ran anywhere). A migration the branch itself added is new at the merge base, so editing it is never caught. `pnpm check:sql` (squawk, via `scripts/check-migrations.mjs`) lints the ones a change ADDS — the applied history holds 1322 findings that stay, because rewriting an applied file is a worse hazard than any of them. Whether a file runs inside a transaction is a property of the FILE (the `migrate: no-transaction` marker, which must be a comment line of its OWN — quoted inside a sentence it does not count, and used to), so the check runs two passes; `.squawk.toml` carries the argument. Suppress a rule with `-- squawk-ignore <rule-name>` on the line before the statement, with the reason beside it.
- Dead JS code and unused dependencies: `pnpm check:dead` (knip, the `dead-code` CI job) gates **everything knip reports — files, dependencies, binaries AND exports** across `web/`, `design-system/` and the root scripts. An exported name nothing imports fails the build, an exported *type* included; `knip.config.js` argues why the line ended up there rather than at the usual "report exports, do not gate them", and the two deliberate exemptions carry a JSDoc `@public` tag beside the code rather than an entry in the config. (This paragraph used to describe a narrower gate and a `pnpm check:dead:exports` script that no longer exists — the config is the current answer.) CI only, since knip needs both packages' dependencies installed and a fresh worktree usually has neither.
- Dead **Go** code: `deadcode -test -tags=integration,llmlive ./...` (the `dead-code-go` CI job), which builds the whole call graph and reports what no binary and no test can reach. **Both flags are load-bearing.** Without the tags it does not parse the build-tagged tests and calls their helpers dead — the same blindness `.golangci.yml`'s `run.build-tags` documents for depguard. Without `-test` every test-only helper is dead. The residue it cannot see is a function whose only caller is its own test; that one still needs a reviewer. To run it locally: `go install golang.org/x/tools/cmd/deadcode@v0.52.0` — with a toolchain at least as new as go.mod's `go` directive, or the binary's type checker rejects the module and reports "package requires newer Go version" instead of findings.
- `.github/workflows/` — linted by `actionlint` (the `workflows` CI job), which also runs shellcheck over every `run:` block, and audited by `zizmor --offline` in the same job. Every `uses:` is pinned to a commit SHA with a `# vX.Y.Z` comment, which Dependabot moves together; a new action goes in the same way or zizmor fails. `actions/checkout` takes `persist-credentials: false` unless a later step pushes. A finding wrong for one line carries `# zizmor: ignore[<audit>]` with the reason beside it. Nothing else reads a workflow until GitHub runs it, so a mistake in one that fires on a tag or a schedule surfaces late.
- **The instruction files are linted** by `agnix --strict` (the `docs` CI job): an unclosed code fence turns the rest of an AGENTS.md into one code block, and that is invisible to a reader skimming the rendered file. `.agnix.toml` argues every rule it turns off.
- **Markdown structure is linted by [rumdl](https://github.com/rvben/rumdl)** (the `docs` CI job, a pre-commit hook on staged `*.md` that steps aside when the binary is missing, `.mise.toml` pins the version). It catches what a link check and a skim do not: an unclosed code fence, a skipped heading level, a malformed table or list, a duplicate heading. `.rumdl.toml` excludes `openspec/changes/archive/**` and the generated `docs/API*.md`, and argues every rule it turns off; turn a rule off there, with the reason, rather than per file. `rumdl fmt` repairs only some findings and gets others wrong (it turns a wrapped line beginning `#659` into a heading), so run `rumdl check` and read the diff of a `fmt`.
- **Syntax rules: ast-grep** (`sgconfig.yml`, the `ast-grep` CI job, a pre-commit hook, `mise run check:ast-grep`). A rule from this file that the code's SHAPE can show — no types needed — gets a rule in `ast-grep/rules/<id>.yml` and a test in `ast-grep/tests/<id>-test.yml` with `valid` and `invalid` snippets; `ast-grep test -U` refreshes the snapshots beside them. The rule's `message` names what is wrong and how to fix it, its `note` says why. A rule is turned on with every violation fixed, never with a baseline. A `.svelte` file is read as HTML whose `<script>` is TypeScript, so a TypeScript rule covers the components too. A rule that needs types stays in golangci-lint or a Go test (the `llmkey` binding, the layering table).
- **The public API contract is checked against main.** `scripts/check-openapi-breaking.sh` (the `api-breaking` CI job) runs oasdiff over `web/static/openapi.yaml` and fails on a change that would break an existing client — a removed field, a narrowed bound, a weaker response promise. A deliberate break passes with an `API-Break: <reason>` commit trailer.
- **Changed lines must be covered by tests** (pull requests only). `.github/scripts/diff-cover-gate.sh` runs [diff-cover](https://github.com/Bachmann1234/diff_cover) against `origin/main` and fails when too small a share of the lines the PR changes is executed by a test; the old uncovered backlog blocks nothing. Go runs in the `backend` aggregator job, the only place every shard's profile is visible: the unit and integration profiles are each converted to Cobertura and merged, so a line covered in any shard counts. Web runs as a step of `web-work`, on vitest's lcov. Excluded: `internal/platform/db` and files with a "Code generated" header, `cmd/*/main.go`, and on the web side tests, specs, `.d.ts`, `generated/` and `fixtures/` (the vitest `coverage.exclude`). A changed line missing from a report counts as not measured, not as uncovered, so both sides list every source file in their report — Go includes a package with no test files at 0%, and `web/vitest.config.ts` sets `coverage.include` so a file no test imports shows at 0%; keep both true. The thresholds are `GO_FAIL_UNDER` (60) and `WEB_FAIL_UNDER` (0, report-only) at the top of the script, picked from the last ~20 merged PRs (nine measurable for Go: three would have failed at 60 and at 70, five at 80); web stays at 0 because three of the four measurable PRs failed even at 60, so it needs a larger sample. There is no commit-trailer escape hatch: get past it by writing the test, or, when the lines cannot honestly be tested, by changing the threshold or the exclusions in the script in the same PR with the reason in review. diff-cover is installed from `.github/requirements/diff-cover.txt` (hash-pinned, in a virtualenv); `COVERAGE_FAIL_UNDER=<n>` overrides both thresholds for one local run.
- **The links in these documents are checked.** `pnpm check:links` (the `docs` CI job and a pre-commit hook) resolves every relative Markdown link in the repository. The table below is the map an agent is meant to follow, so a link that no longer resolves does not just fail — it sends the reader somewhere confidently. The hook carries no glob on purpose: what breaks a link is renaming its **target**, usually a `.go` or `.svelte` file.
- **What we hand to someone else is linted too** (the `artifacts` CI job): `web/static/install.sh` is served at `freehire.me/install.sh` to be piped into `sh`, so shellcheck covers every tracked `*.sh`; both Dockerfiles go through hadolint (`.hadolint.yaml` argues its two exclusions, and a finding wrong for one line carries `# hadolint ignore=` beside it); and `web/static/openapi.yaml` is validated against the OpenAPI specification, since it is a static asset nothing else parses.
- **There is no `sources/` directory.** The board catalog is the `boards` table (`internal/ingest/boardcatalog`) and the Telegram channel list is `telegram_channels` — both moved out of YAML, and nothing in the repository parses a board file any more. `internal/ingest/sources` is the *adapters*, not the catalog.
- `design-system/` — a separate pnpm package, sibling to `web/` and `extension/`, linked via
  pnpm's `link:../design-system` (`web/`) or npm's `file:../design-system` (`extension/`) — both
  are symlinks, not copies. **Install it before building either consumer** — neither package
  manager installs a linked/`file:` package's own dependencies for you. Being a symlink also
  means each of the three installs its OWN `node_modules`, so a package all of them depend on
  sits on disk more than once — free for a value, fatal for a TYPE, since TypeScript
  identifies a type by where it was declared and two copies of `LucideIcon` are two unrelated
  types sharing a name. `pnpm check:deps` (`scripts/check-shared-deps.mjs`, the `shared-deps`
  CI job) holds the three manifests to the same version for exactly the packages the design
  system imports as types — a set it DERIVES from `design-system/src` rather than listing, so
  a new type import joins the rule by existing. It reads no lockfile and compares the declared
  range literally: `^1.25.0` and `^1.45.0` overlap perfectly well and are the two ranges that
  put `@lucide/svelte` on disk twice (freehire#2836), so "the ranges are compatible" is the
  wrong question. `peerDependencies` is exempt — a library's peer range is deliberately wider
  than what it develops against. Dependabot cannot see any of this: it reads three
  `package.json` files as three unrelated projects, so it will keep proposing a bump to one of
  them, and that PR is correct once the other two move with it.
- `extension/` — the browser extension (WXT + Svelte side-panel agent client), npm-managed
  unlike the rest of the JS in this repo. See [extension/AGENTS.md](extension/AGENTS.md).
- `internal/platform/db/` — **generated**; edit `internal/platform/db/queries/*.sql` and run `make sqlc`. The pre-commit hook and the `sqlc` CI job regenerate and diff, so a query edited without regenerating no longer ships the old Go with every check green. Both use `make sqlc`, which holds the only version pin — a second pin would be a second answer, and the drift between them would look exactly like stale code.
- `services/pii-filter` — a standalone service, not a Go package.
- **There is no `deploy/` snapshot of the host any more.** The systemd units, the nginx
  snippets, the operator scripts (`release.sh`, `autodeploy.sh`, `gen-ingest-timers.sh`)
  and the prose explaining them all live in the private `freehire-ops` repository, under
  `provision/host2/` and `scripts/host2/` — that is the only place any of them is edited,
  and `provision/host2/AGENTS.md` is the document to read about this machine. This repo
  carried a third copy of all of it until 2026-09-18. It drifted from the host in BOTH
  directions and was deleted rather than kept in step: measured against the running host,
  this repo's copies were the more accurate ones for the scripts (14 of 17), for most units
  and for all three hand-edited nginx files, while a Sentry gate merged here had never once
  run because the host had never heard of it. Both being partly right, with nothing able to
  say which half, is the whole argument for one copy. `deploy/rollback/` stays: those are a
  migration's SQL, not host configuration.
- **Nothing in `freehire-ops` deploys itself either.** `release.sh` builds and flips the
  app; it never touches a unit, a snippet or a script. Editing one there is half the job —
  the other half is copying it to the host and running `systemctl daemon-reload`. Its own
  `scripts/host2/drift-check.sh` reports what has moved apart.

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
`:18xxx` in a worktree), so two agents in two worktrees never migrate or serve over each
other. `mise run db:up` starts that Postgres and creates and migrates this checkout's
database; `db:drop` and `db:psql` do what they say. The same environment sets
`TESTDB_ADMIN_URL`, so integration tests migrate a template there instead of starting a
container per test binary — run `db:up` before them, and run them where mise's environment
reaches: a shell with `eval "$(mise activate zsh)"`, or `mise exec -- go test -tags=integration ./...`.

**Before committing** any `*.go` file: `gofmt -w` those paths (`gofmt -l .` must print
nothing), then `go vet ./...` and `go test ./...`. Do not commit if they fail. Skip this
suite when the commit has no Go. Integration-tagged tests stay push-time (`go vet
-tags=integration ./...` before every push; the full tagged suite when behaviour changed).

**Pre-commit hooks:** [lefthook](https://github.com/evilmartians/lefthook) (`go install
github.com/evilmartians/lefthook@latest`, then `lefthook install` once per clone) runs
gofmt/vet/golangci-lint on staged Go files and eslint on staged files in web/, extension/,
design-system/ — the same ratchet policy as CI (only new issues fail the commit), so it
won't block on the pre-existing backlog.

**Secrets** are the one thing the hooks *prevent* rather than report: `gitleaks` (`brew
install gitleaks`) scans the staged index on every commit and fails hard if the binary is
missing, because a scanner that quietly skips makes a commit look examined. The
`gitleaks` workflow scans the whole history on top, since `--no-verify` walks past the
hook and a credential removed in a later commit is still leaked. A finding that survives
`.gitleaks.toml` means **revoke the credential** — rewriting history does not un-leak
anything. Suppressions in that file allowlist a *line shape*, never a path: gitleaks ORs
`paths` with `regexes` and prunes the file before reading it, so a path entry switches
the scanner off for that file entirely.

**`go test ./...` compiles no `//go:build integration` file, and those files are not
confined to `internal/platform/db`** — there are 187 of them across 20 packages, and `internal/api/handler`
holds 78, which call unexported constructors like `newCVHandlers`. A changed signature
therefore passes every command above except the `vet` line, then fails CI, which runs
`go test -tags=integration ./...` over the whole module. The vet line is the cheap guard:
seconds, no Docker. Run the full tagged suite when you change behaviour rather than a
signature.

**Before you run, change or add a `cmd/<name>` worker, read its entry in [docs/agents/workers.md](docs/agents/workers.md) when it has one**: what it needs, when it runs, and the trap behind it. Not every worker has an entry yet (`ls cmd/` is the full list); without one, read the worker's own `main.go`. A new worker gets its entry there.

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
| **Worker commands** (each `cmd/<name>`: what it needs, when it runs, its trap) | [docs/agents/workers.md](docs/agents/workers.md) |
| **Company facets** (remote_regions vs yc_* curated facets) | [docs/agents/company-facets.md](docs/agents/company-facets.md) |
| **SPA sub-context** (SvelteKit, auth flow, API conventions) | [web/AGENTS.md](web/AGENTS.md) |
| **Host configuration** (systemd units and timers, operator scripts, the two-file env split) — NOT in this repo | the private `freehire-ops`: `provision/host2/AGENTS.md` |
| **Design system** (pnpm package, tokens, Tailwind @source trap) | [design-system/AGENTS.md](design-system/AGENTS.md) |
| **`internal/platform`** — the block itself: what it is, what it may import | [internal/platform/AGENTS.md](internal/platform/AGENTS.md) |
| **SQL layer** (sqlc, queries, migrations) | [internal/platform/db/AGENTS.md](internal/platform/db/AGENTS.md) |
| **Cron worker plumbing** (Main/Bootstrap, exit codes, heartbeat, corruption-tolerant scans) | [internal/platform/worker/AGENTS.md](internal/platform/worker/AGENTS.md) |
| **LLM client** (provider-agnostic wrapper, schema cache, streaming, attribution tags) | [internal/platform/llm/AGENTS.md](internal/platform/llm/AGENTS.md) |
| **Headless browser** (launching Chrome, in-page fetch, the stealth flags' single home) | [internal/platform/browser/AGENTS.md](internal/platform/browser/AGENTS.md) |
| **Scrapling fetch tier** (the self-hosted service for sources no plain client reaches: modes, sessions, the transport) | [internal/platform/scraplingfetch/AGENTS.md](internal/platform/scraplingfetch/AGENTS.md) |
| **Exception logs and metrics** (OpenTelemetry for backend, workers, frontend — env-gated; Prometheus) | [internal/platform/observability/AGENTS.md](internal/platform/observability/AGENTS.md) |
| **`internal/dict`** — the block itself: what it is, what it may import | [internal/dict/AGENTS.md](internal/dict/AGENTS.md) |
| **Company names** (real display names for slug-named companies) | [internal/dict/companyname/AGENTS.md](internal/dict/companyname/AGENTS.md) |
| **Enrichment** (Enrichment contract, LLM Provider; enums live in `internal/dict/vocab`) | [internal/ai/enrich/AGENTS.md](internal/ai/enrich/AGENTS.md) |
| **Geography** (country/region codes, work-mode hint, dict-only vs hybrid) | [internal/dict/location/AGENTS.md](internal/dict/location/AGENTS.md) |
| **Skill tagging** (alias→canonical dictionary, jobs.skills facet) | [internal/dict/skilltag/AGENTS.md](internal/dict/skilltag/AGENTS.md) |
| **Answer topics** (a screening question keyed by what it asks, not how it is worded) | [internal/dict/answertopic/AGENTS.md](internal/dict/answertopic/AGENTS.md) |
| **Skill vectors** (the permanent position registry, the ballast, why coverage decides the order) | [internal/dict/skillvec/AGENTS.md](internal/dict/skillvec/AGENTS.md) |
| **Seniority & category** (title→seniority/category, dict-only) | [internal/dict/classify/AGENTS.md](internal/dict/classify/AGENTS.md) |
| **`internal/ai`** — the block itself: what it is, what it may import | [internal/ai/AGENTS.md](internal/ai/AGENTS.md) |
| **Browser tools** (relays tool frames between agent harness and extension) | [internal/ai/browsertools/AGENTS.md](internal/ai/browsertools/AGENTS.md) |
| **Semantic embedding** (semantic_outbox, incremental chunked embeds into pgvector) | [internal/ai/embed/AGENTS.md](internal/ai/embed/AGENTS.md) |
| **In-app assistant** (turn loop, tool registry, presets, transcripts) | [internal/ai/assistant/AGENTS.md](internal/ai/assistant/AGENTS.md) |
| **Speech to text** (dictation into the composer, the filename rule, spend bounds) | [internal/ai/speech/AGENTS.md](internal/ai/speech/AGENTS.md) |
| **LLM spend attribution** (per-user gateway credential, feature tags, fail-open) | [internal/ai/llmkey/AGENTS.md](internal/ai/llmkey/AGENTS.md) |
| **Plan limits** (the daily per-feature allowance, the two tailoring bounds, shadow mode) | [internal/ai/plan/AGENTS.md](internal/ai/plan/AGENTS.md) |
| **`internal/identity`** — the block itself: what it is, what it may import | [internal/identity/AGENTS.md](internal/identity/AGENTS.md) |
| **Accounts** (identity resolution, seizure rule, mailed codes) | [internal/identity/accounts/AGENTS.md](internal/identity/accounts/AGENTS.md) |
| **Auth primitives** (JWT, API keys, cookie transport, middleware) | [internal/identity/auth/AGENTS.md](internal/identity/auth/AGENTS.md) |
| **OAuth sign-in** (provider registry, state cookie, identity resolution) | [internal/identity/auth/oauth/AGENTS.md](internal/identity/auth/oauth/AGENTS.md) |
| **Discounts** (promo codes, invite links, the reward earned by an invoice that collected) | [internal/identity/promo/AGENTS.md](internal/identity/promo/AGENTS.md) |
| **`internal/candidate`** — the block itself: what it is, what it may import | [internal/candidate/AGENTS.md](internal/candidate/AGENTS.md) |
| **AI fit analysis** (three-stage LLM prompt-chain, score, verdict, stream) | [internal/candidate/matchanalysis/AGENTS.md](internal/candidate/matchanalysis/AGENTS.md) |
| **Fit-analysis use cases** (the cache, the staleness stamp, the credit rule, coalescing) | [internal/candidate/fitanalysis/AGENTS.md](internal/candidate/fitanalysis/AGENTS.md) |
| **Job-match scoring** (deterministic CV-vs-vacancy score, the unverifiable rule) | [internal/candidate/cvmatch/AGENTS.md](internal/candidate/cvmatch/AGENTS.md) |
| **ATS-readiness score** (deterministic CV score, de-identified LLM review, delta) | [internal/candidate/atscheck/AGENTS.md](internal/candidate/atscheck/AGENTS.md) |
| **Experience bank** (durable employments + evidence atoms, provenance, retrieval) | [internal/candidate/experience/AGENTS.md](internal/candidate/experience/AGENTS.md) |
| **Screening answer bank** (what the candidate answers employers, accumulating across applications) | [internal/candidate/answerbank/AGENTS.md](internal/candidate/answerbank/AGENTS.md) |
| **Résumé identity** (one stored CV per user, contact-block layers) | [internal/candidate/resume/AGENTS.md](internal/candidate/resume/AGENTS.md) |
| **Talent Network** (the public anonymous catalogue: the dictionary-only projection, the minted handle, the snapshot) | [internal/candidate/talentnetwork/AGENTS.md](internal/candidate/talentnetwork/AGENTS.md) |
| **Structured CV** (LLM parse of stored CV, stamp-and-compare) | [internal/candidate/resumeextract/AGENTS.md](internal/candidate/resumeextract/AGENTS.md) |
| **PII masking** (fail-closed CV→LLM redaction, reversible placeholders) | [internal/candidate/pii/AGENTS.md](internal/candidate/pii/AGENTS.md) |
| **Cover letters** (three-stage chain, the provenance gate, the vacancy's language, the audit floor) | [internal/candidate/coverletter/AGENTS.md](internal/candidate/coverletter/AGENTS.md) |
| **CV rendering** (templates, fonts, previews) | [internal/candidate/cv/AGENTS.md](internal/candidate/cv/AGENTS.md) |
| **CV edits** (the only writer: path operations, revisions, undo, the evidence gate) | [internal/candidate/cvedit/AGENTS.md](internal/candidate/cvedit/AGENTS.md) |
| **`internal/job`** — the block itself: what it is, what it may import | [internal/job/AGENTS.md](internal/job/AGENTS.md) |
| **Job wire shape** (the single public projection of a job) | [internal/job/jobview/AGENTS.md](internal/job/jobview/AGENTS.md) |
| **Job fingerprints** (content_hash vs role_fingerprint vs RoleKey — which hash for which job) | [internal/job/jobhash/AGENTS.md](internal/job/jobhash/AGENTS.md) |
| **Requirements extraction** (a posting's own requirements from its markup, no model; three closed vocabularies) | [internal/job/reqextract/AGENTS.md](internal/job/reqextract/AGENTS.md) |
| **Ghost detection** (hedged posting-reality verdict, two evidence tiers, crosscheck) | [internal/job/ghost/AGENTS.md](internal/job/ghost/AGENTS.md) |
| **Search announcements** (Indexing API + IndexNow, the ledger, the budget day) | [internal/job/searchping/AGENTS.md](internal/job/searchping/AGENTS.md) |
| **YC directory** (import-yc, curated facets, matching by former names) | [internal/job/ycdir/AGENTS.md](internal/job/ycdir/AGENTS.md) |
| **Wikidata company matching** (type-confidence gate, no keyword scan, backfill-company-info-wikipedia) | [internal/job/wikicompany/AGENTS.md](internal/job/wikicompany/AGENTS.md) |
| **Company collections** (curated company tags, register datasets, reconciliation) | [internal/job/collections/AGENTS.md](internal/job/collections/AGENTS.md) |
| **`internal/application`** — the block itself: what it is, what it may import | [internal/application/AGENTS.md](internal/application/AGENTS.md) |
| **Per-user job tracking** (view/apply/save/track, stages, /me/tracking) | [internal/application/userjob/AGENTS.md](internal/application/userjob/AGENTS.md) |
| **View counts** (nginx access logs → per-job views) | [internal/application/viewlog/AGENTS.md](internal/application/viewlog/AGENTS.md) |
| **Agent views** (postings served to agents, the day-file log, the scope that keeps people out) | [internal/application/agentview/AGENTS.md](internal/application/agentview/AGENTS.md) |
| **Auto-apply queue** (queue-drain domain logic for unattended submission) | [internal/application/autoapply/AGENTS.md](internal/application/autoapply/AGENTS.md) |
| **`internal/search`** — the block itself: what it is, what it may import | [internal/search/AGENTS.md](internal/search/AGENTS.md) |
| **Search** (Meili index topology, rebuild swap, reindex hazards) | [internal/search/AGENTS.md](internal/search/AGENTS.md) |
| **Facet-search drain** (search_outbox, incremental facet-index pushes, reconciler) | [internal/search/searchdrain/AGENTS.md](internal/search/searchdrain/AGENTS.md) |
| **`internal/ingest`** — the block itself: what it is, what it may import | [internal/ingest/AGENTS.md](internal/ingest/AGENTS.md) |
| **Source ingest** (adapters, provider registry, validation) | [internal/ingest/sources/AGENTS.md](internal/ingest/sources/AGENTS.md) |
| **Ingest scheduling** (what makes a crawl due, the claim, the fleet cap, the measurement behind every override) | [internal/ingest/ingestsched/AGENTS.md](internal/ingest/ingestsched/AGENTS.md) |
| **Board catalog** (the `boards` table: lifecycle, insert-time validation, `cmd/add-board`) | [internal/ingest/boardcatalog/AGENTS.md](internal/ingest/boardcatalog/AGENTS.md) |
| **Pipeline** (Runner, dedup, UpsertJob, board health, search indexing) | [internal/ingest/pipeline/AGENTS.md](internal/ingest/pipeline/AGENTS.md) |
| **Apply-form capture** (ATS application forms, verbatim platform vocabulary, queue drain) | [internal/ingest/applyform/AGENTS.md](internal/ingest/applyform/AGENTS.md) |
| **Screening answers** (the six repeating ATS questions no CV can supply, measured against captured forms) | [internal/ingest/screeninganswers/AGENTS.md](internal/ingest/screeninganswers/AGENTS.md) |
| **Link resolution** (outbound job URL → destination's own identity) | [internal/ingest/linksource/AGENTS.md](internal/ingest/linksource/AGENTS.md) |
| **ATS board recognition** (URL → (source, board), shared conventions) | [internal/ingest/atsboard/AGENTS.md](internal/ingest/atsboard/AGENTS.md) |
| **Board contributions** (crowdsourced URL → (source, board) onboarding) | [internal/ingest/contribution/AGENTS.md](internal/ingest/contribution/AGENTS.md) |
| **Employer accounts** (company claim, work-email verification, self-service vacancy authoring) | [internal/ingest/employer/AGENTS.md](internal/ingest/employer/AGENTS.md) |
| **On-site applications** (a candidate applying to an employer vacancy here, recorded once, emailed to the employer) | [internal/ingest/directapply/AGENTS.md](internal/ingest/directapply/AGENTS.md) |
| **Telegram** (crawl + LLM vacancy extraction) | [internal/ingest/telegram/AGENTS.md](internal/ingest/telegram/AGENTS.md) |
| **`internal/engage`** — the block itself: what it is, what it may import | [internal/engage/AGENTS.md](internal/engage/AGENTS.md) |
| **Interview buddies** (anonymous peer network: handle, feed vocabulary, one conversation per pair, mock interviews) | [internal/engage/buddy/AGENTS.md](internal/engage/buddy/AGENTS.md) |
| **Messaging core** (conversations of every kind: participants speaking as personas, request-and-accept, account-wide blocks, reports, unread notices) | [internal/engage/messaging/AGENTS.md](internal/engage/messaging/AGENTS.md) |
| **Employee referrals** (offer/request marketplace, moderation, anonymity) | [internal/engage/referral/AGENTS.md](internal/engage/referral/AGENTS.md) |
| **Mentorship marketplace** (the slot engine, the timezone rules, the non-overlap constraint) | [internal/engage/mentorship/AGENTS.md](internal/engage/mentorship/AGENTS.md) |
| **Daily social digest** (the most-viewed list, the editorial rules, the publisher seam) | [internal/engage/socialdigest/AGENTS.md](internal/engage/socialdigest/AGENTS.md) |
| **LinkedIn credential** (the 60-day token, the refresh that may never be granted, the warning) | [internal/engage/linkedinauth/AGENTS.md](internal/engage/linkedinauth/AGENTS.md) |
| **Discord paid channels** (the account link, the one role, the rotating reconciliation queue) | [internal/engage/discordlink/AGENTS.md](internal/engage/discordlink/AGENTS.md) |
| **`internal/api`** — the block itself: what it is, what it may import | [internal/api/AGENTS.md](internal/api/AGENTS.md) |
| **HTTP handlers** (response shapes, error rendering, routes) | [internal/api/handler/AGENTS.md](internal/api/handler/AGENTS.md) |
| **Auto-apply browser driver** (headless Chrome: DOM scan, reconcile, resolve, fill/submit) | [internal/api/atsapply/AGENTS.md](internal/api/atsapply/AGENTS.md) |
| **ChatGPT app** (the SECOND MCP server: why it is not the OJCP one, the annotations, the dropped filter) | [internal/api/mcpapp/AGENTS.md](internal/api/mcpapp/AGENTS.md) |
| **OJCP provider** (the agent-facing standard: the projection, both transports, the manifest) | [internal/api/ojcp/AGENTS.md](internal/api/ojcp/AGENTS.md) |

## Conventions

- **Response shapes:** Lists: `{"data": ..., "meta": {...}}`; single items: `{"data": ...}`; errors: `{"error": msg}`
- **A dropped filter says so:** any endpoint whose answer WIDENS when it does not understand a query param reports the params it did not read, in `meta.ignored_params` (omitted entirely when there are none). Refusing them instead would break saved searches and shared links that still carry a retired facet; staying silent is what let a mistyped `country=it` pass for a search of Italy. `search.UnknownParams` owns the SEARCH filter's vocabulary and only that, so an endpoint reading a different one says so itself — `/companies` through `UnknownCompanyParams`, `/jobs` (which reads no filter at all) through its own two-param list. Its value-level twin is `meta.ignored_values`: a closed-vocabulary facet value outside `internal/dict/vocab` (`work_mode=foo`) still narrows — to nothing — and `search.IgnoredValues` names it rather than letting `total: 0` pass for an empty market. The same rule decides what may be SUBSCRIBED to: `search.Narrows` asks whether a saved search selects less than the catalogue, and a subscription to one that does not is refused rather than reported, because nobody reads a digest's meta
- **Where a sign-up came from:** the browser reports the visit that led to a brand-new account — campaign tags, the referring HOST, the landing PATH, never a full URL — to `POST /api/v1/me/attribution` (`internal/api/handler/me_attribution.go`), which keeps the first report per account in `user_signup_attribution` (migration 0228), for accounts under ten minutes old only. This is the complete count and does not depend on analytics consent; Google Analytics only sees the visitors who accepted cookies (on 2026-11-02 it saw 228 of 644 sign-ups). The same call sends the GA4 `sign_up` from the server through the Measurement Protocol (`internal/engage/ga4mp`, needs `GA4_MEASUREMENT_ID` + `GA4_API_SECRET`, inert without them) and answers `{"data":{"ga4_sent":bool}}`; the browser sends `sign_up` itself ONLY when that is false, because GA4 has no event-level dedup and both sending would count the account twice. Never send a client id the visitor did not give: it exists only after they accepted analytics. Source report: `SELECT coalesce(a.utm_source, a.referrer_host, '(direct)') src, count(*) FROM users u LEFT JOIN user_signup_attribution a ON a.user_id = u.id WHERE u.created_at >= :day GROUP BY 1 ORDER BY 2 DESC` — a LEFT JOIN, since accounts from before 0228, from mobile, or whose browser could not report have no row.
- **Dedup key:** `jobs.UNIQUE (source, external_id)` — `UpsertJob` is `ON CONFLICT` on it
- **Company key:** `normalize.CompanySlug`, never `normalize.Slug` — the corporate form is not part of who the employer is. One legal-form vocabulary exists (`internal/dict/normalize`) and a test walks the module to keep it that way; there were four, they disagreed, and every resulting miss was silent. Spellings a rule cannot collapse (`dollartree` vs `dollar-tree`) resolve through `company_slug_aliases`, the one company-adjacent table that is NOT derived from `jobs`
- **Auth:** JWT in httpOnly cookie, same-origin, carrying the account's `token_version` so sessions are revocable. `RequireAuth` (cookie only) / `RequireAuthOrKey` (cookie or full-scope Bearer) / `RequireAuthOrScopedKey` (also admits a narrow key)
- **Email ownership:** `users.email_verified`; a password registration starts unverified and is confirmed by a mailed six-digit code. An unverified, password-backed account is **seized** (password cleared, sessions revoked, API keys deleted) when a provider-verified OAuth identity arrives for its address — the account-pre-hijacking defence
- **API keys:** Hashed at rest (SHA-256), scoped `full` or `cv`, and mintable only by an account with a verified address. Key management (create/list/revoke) and password change are cookie-only. A key does not carry the session generation, so a `token_version` bump does not revoke it — that is intentional for sign-out-everywhere and wrong for a takeover, so the seizure and the mailed-code password reset delete the rows in the same statement
- **Enrichment:** Queue-driven (`enrichment_outbox`), provider-agnostic LLM, `Sanitize` + `Validate` gate
- **Embeddings:** Queue-driven (`semantic_outbox`), incremental (`cmd/embed`) — pgvector-backed `job_semantic_chunks` plus a legacy single-vector column, no search index. Reconciled by bumping the embedder-model version string, which re-enqueues the whole catalogue through the existing staleness check
- **Catalogue scale:** Every public figure describing how big the catalogue is — `GET /api/v1/stats/catalog`, the jobs list's `meta.total`, the `/about` and `/open` strips — reads ONE snapshot published by `cmd/rollup-stats` (`internal/ingest/catalogstats`). Never count on a request path: `catalogstats.Load` takes no exact counter, so it cannot. A read never fails; a cold cache, an unreachable Redis or no cache at all degrades to the planner estimate with `exact: false`, which zeroes the figures that exist only in the database — pass `exact` through to whatever renders them, because a zero must not reach a page as if it were a measurement. The snapshot's `open_jobs` is the raw count (every open posting, reposts across boards included, never duplicate-suppressed) — `unique_open_jobs` is the de-duplicated figure Meilisearch's own index holds, what `/jobs` itself would show unfiltered. It is measured the same way as `telegram_channels`: `cmd/rollup-stats` makes one `search.Client.Search` call (limit 1, no filters) before publishing, best-effort — a missing `MEILI_MASTER_KEY` or an unreachable Meilisearch costs that one figure, not the snapshot
- **Per-source figures:** `GET /api/v1/sources` and the public `/sources` page read ONE snapshot, `source_stats`, rebuilt by a pass inside `cmd/rollup-stats` (`internal/ingest/sourcestats`) as an atomic delete-and-reinsert. It carries the same two-count pair `catalogstats` does and for the same reason: `open_jobs` is the raw open count — duplicates included — and is the denominator the ATS-overlap figures are arithmetic on, while `browsable_jobs` is Meilisearch's de-duplicated count, which is what `/jobs?source=<key>` will show and therefore what the PAGE displays. A card whose number the search it links to contradicts is the trap the pair exists to avoid. **`browsable_jobs` is NULL when Meilisearch could not be measured, never 0** — an unreachable index costs that one figure, not the run, and a 0 would state on a public page that every source carries nothing. The snapshot's spine is `sourcestats.Union` over THREE sets, each covering a hole the others leave: the adapter registry (without it a source with postings and no adapter — `telegram`, an extraction pipeline absent from `sources.Taxonomy()` — is dropped), this run's scan (without it every adapter whose postings have all closed is dropped, which is the one case the page most needs to state), and the KEYS of the snapshot being replaced (without it a source in neither of the first two vanishes the moment it goes quiet — the same silent drop, one closure later). Only names carry over; every figure is re-measured. `GET /api/v1/sources` calls the same `Union` rather than keeping its own copy of the rule. The **ATS-overlap** figure counts rows the dedup pass matched to a first-party ATS posting (`jobs.duplicate_of_aggregator`); the remainder is `ats_unmatched` and is **never** called exclusive, on the wire or on the page, because the pass finding no pair is the absence of evidence — two tests enforce the wording — one over the rendered card, one over the source of every file on the surface, including the page's own prose. It is carried for aggregators ONLY: the suppression pass never marks any other row, so a 0 there is arithmetic and would read as "fully exclusive". The aggregate scan reads no `description` column (a guard test on the generated SQL) and excludes `is_private`. **Source logos are resolved in the browser from the source's DISPLAY NAME** (`sourceLogoUrl`), never from a posting host: the first version sampled `min(url)` per source and served the WRONG brand, because an ATS posting's URL usually lives on the employer's own domain — measured on prod, `greenhouse` sampled `bankrate.com` and `successfactors` a staffing agency — while a per-tenant subdomain (`jobs.smartrecruiters.com`) simply 404s. By name, 14 of 15 resolve, and 7 of 8 sampled sources return the byte-identical image whether asked by name or by an explicit `?domain=` — a platform brand is far less ambiguous than an employer name. **That margin is not free, though**: the proxy answers a name it resolves WRONGLY with a 200, so a mismatch is invisible to every consumer (see the `g2i` case behind `cmd/publish-logo-domains`). **`SOURCE_LOGO_DOMAINS` (web/src/lib/logo.ts) is that escape hatch, curated and measured**: of the 145 sources carrying 500+ jobs, 14 had no logo by name and all 14 resolve with the platform's own domain passed as `?domain=`, which wins over the proxy's name map. `successfactors` settles why a nicer NAME was never the whole fix — its label was already correct and the proxy still had nothing. It is an OVERRIDE, not a registry: a source missing from it resolves by name as before, so the map going stale costs a logo rather than breaking one — the deliberate difference from the posting-host sampling it replaced, which was wrong for every source it covered. A curated domain implies a curated display name, and a test holds the two maps together. A miss still 404s, so every call site renders through the `EntityLogo` primitive rather than a hand-rolled `<img>`: besides the `onerror` fallback it catches a 404 that landed BEFORE hydration, which a server-rendered page needs precisely because the browser fetches the image while no handler exists yet. **A user's `excluded_sources` are seeded into the search filter by `filtersFromProfile`**, the same function the saved-search alert is built from — so avoiding a source from the job page's provenance row (`JobSourceRow`) reaches both the feed and the digest. Before that seeding the list was stored, editable in the profile, and read by nobody
- **Dictionaries:** All facet dictionaries are dict-only in production — never guess, emit nothing for unknowns
- **View counts:** anything describing how much a posting was LOOKED AT reads `job_daily_views.page_uniques`, **never `uniques`**. `uniques` fuses bot-filtered page opens with UNFILTERED API reads, and crawlers are most of this host's traffic, so a figure built on it reports what robots fetched. The rule used to live only in `social-digest`, which is why `/api/v1/stats/engagement` and the `/open` grid published 11,027,722 against the real 5,401,347 for months (freehire#2945) — a rule held by one reader is not a rule. Seed the two columns to DIFFERENT totals in any test that asserts on a view figure: `page_uniques` carries a `DEFAULT 0`, so a test seeding only `uniques` passes whichever column the query reads. `page_uniques` is also every VISITOR, signed in or not, so it never belongs beside signed-in counters without its label saying so. `job_daily_views.agent_uniques` counts postings served to agents (MCP, OJCP, `/agent/jobs/search`, via `internal/application/agentview`); `jobs.view_count` = `uniques` + `agent_uniques`. `jobs.view_count` therefore mixes people, API reads and agents: it is a total-reach figure (the most-viewed sort and the job page show it as such), and the only human-only figure remains `page_uniques`
- **Job deletion:** The lifecycle only soft-closes, and nothing hard-deletes jobs. `cmd/prune` used to, stripping postings that did not belong on an IT board; it was removed once the catalogue became broader than IT. The rows it took are archived in `pruned_jobs` (kept), and the applications it orphaned still carry a NULL `job_id`, which the tracking code handles
- **In-app assistant:** a bounded tool-calling loop in-process (`internal/ai/assistant`), streamed over SSE, open to every signed-in user. Tools act as the authenticated caller — no credential is minted for an agent
- **No free AI — the paid gate:** every model call made for a user's action needs a live Pro or Ultra entitlement — with THREE named exceptions that every account gets (below): parsing a stored CV, the search interpretation (`search_intent.go`) and the support agent. That is a product invariant, not a quota: there is no free allowance and no override (`PLAN_FREE_DAILY_<F>` is gone and no longer read), and `plan.decide` refuses an unpaid tier before it reads any allowance. It is enforced twice in `internal/api/handler`: the `mw.paidAI(feature)` middleware (`paid_ai.go`) answers an unpaid caller 402 — in the same body every plan refusal carries, before any stream opens — and an unreadable plan 503, never a pass; and the user-call binding (`llmBinding.bind/caller`, `user_llm.go`) refuses to hand out a model client for a context the gate did not mark (`plan.WithPaidAI`), so a new model-calling route mounted without the middleware fails its first test. A test also forbids `llmkey.Bind/Caller` anywhere in the handler package outside the binding. Callers the middleware cannot reach ask `plan.Store.RequirePaid` themselves: the auto-apply orchestrator's tailor run (for the queue entry's owner) and `cmd/auto-apply`'s drafting (a lapsed candidate's attempt parks). **The CV parse is the exception, and it is a mark, not a missing check**: the upload's background extraction and its retry run for every account under `plan.WithFreeAI`, which the binding accepts beside `plan.WithPaidAI`, on their own cheap model (`RESUME_EXTRACT_MODEL`, falling back to `LLM_MODEL`). It was paid until 2026-10-19 and that cost paying customers: a CV uploaded before upgrading was never parsed, upgrading did not re-run it, and the experience bank every paid feature reads stayed empty — tailoring and the fit analysis failed for somebody who had just paid. **Support is the third mark**: `POST /support/sessions/:id/messages` (`assistant_support.go`) sets `plan.WithFreeAI` and meters itself (`meterSupport`, failing CLOSED — unlike the paid meter, because a counter that cannot be read must not become unbounded spend): a flat `SUPPORT_DAILY_MESSAGES` per account (default 20, 402), `SUPPORT_TURNS_PER_SESSION` per conversation (default 12, 429) and `SUPPORT_DAILY_MESSAGES_GLOBAL` for everybody together (default 2000, 429). `SUPPORT_MODEL` picks its model (falling back to `LLM_MODEL`), `ONBOARDING_REPLY_TO` is the inbox its handover mail goes to. Its refusals never carry an upgrade link, and the generic `/assistant/sessions/:id/...` routes answer a support session 404. Cached AI results, deterministic scoring, search and the public CV roast stay open to free. Ownerless background work (enrichment, embeddings, Telegram) is unaffected.
- **Plan limits:** every metered AI action draws on a per-feature, per-UTC-day allowance (`internal/ai/plan`), on top of the paid gate above. There is no currency and no balance: a paid plan differs in how MUCH of a feature it allows in a day. Tailoring carries two bounds (sessions per day, turns per session) because either alone leaves the hole this replaced. Enforcement is a per-feature switch — every metered feature ships ENFORCING today, so a refusal is real from day one; `PLAN_ENFORCE` remains the lever that turns a FUTURE feature on one at a time after its own shadow run has been read, which is how auto-apply and then the rest arrived at this state without ever sharing a deploy. Refusal is HTTP 402, and on a stream it must precede the stream. **THREE tiers** since migration 0141: free, pro and ultra, resolved by `TierOf` as the better one still live (`users.pro_until` and `users.ultra_until`, each derived from three source columns). `PLAN_PRO_DAILY_<F>` and `PLAN_ULTRA_DAILY_<F>` move a paid tier's figure without a deploy; `STRIPE_ULTRA_PRICE_IDS` is what makes Ultra sellable at all, and unset it simply is not offered. **`auto-apply` was the first feature that shipped ENFORCING**, because pro's ceiling on it is what Ultra is sold on and a ceiling that only counts sells nothing — its route already refused the free tier with 402 before the allowance took that gate's place
- **LLM spend attribution:** every model call made for a signed-in user goes out on that user's OWN gateway credential (`internal/ai/llmkey` — minted lazily on first use, never shown to them) and carries a `feature:` tag. Work that belongs to nobody — enrichment, Telegram, embeddings — keeps the service credential, and a test enforces that background entrypoints never resolve a user's. Attribution fails open: it can never refuse or fail a call. It measures and does not bound — what BOUNDS a turn is the plan (`internal/ai/plan`)
- **Experience provenance:** every banked achievement records whether the CANDIDATE asserted it (`cv_import`/`stated_in_chat`/`manual`) or the MODEL did (`agent_inferred`). Only the former may be written into a CV, and the check lives in the service path, not in a system prompt
- **Error reporting:** OpenTelemetry exception logs (OTLP → Vector → VictoriaLogs), opt-in and env-gated; only genuine faults are reported (see `handler.classify`). There is no Sentry
- **Naming — "CV", not "résumé":** Default new surfaces to **CV**. Don't mass-rename the existing `resume`/`resumeextract` packages and columns — churn without value
