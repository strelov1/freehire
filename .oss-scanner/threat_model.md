# Threat model

Read this together with [SECURITY.md](../SECURITY.md), which carries the project's own scope list, and
[AGENTS.md](../AGENTS.md), which maps the codebase.

## What this project does and where untrusted input enters

freehire is an IT job aggregator served at freehire.me: a Go (Fiber v2) HTTP API backed by PostgreSQL,
Redis and Meilisearch, a SvelteKit app under `web/`, and a fleet of run-once workers under `cmd/`.

Untrusted input enters in three places:

- **The public HTTP API** (`cmd/server`, `internal/api/handler`). Anyone can call it anonymously, a
  signed-in user can call more of it, and moderators can create and edit jobs. Callers include browsers,
  API-key holders (`full` or the narrow `cv` scope), the browser extension, the CLI and AI agents over MCP
  (`internal/api/mcpapp`, `internal/api/ojcp`).
- **Crawled third-party content.** The ingest workers (`cmd/ingest`, `cmd/tg-ingest`, `internal/ingest/...`)
  fetch hundreds of job boards and Telegram channels. Assume every posting is attacker-authored: titles,
  descriptions (HTML), company names, URLs and the links inside them.
- **User uploads and mail.** CV files (PDF, parsed with `pdftotext`, rendered with typst), inbound email
  (`cmd/mail-ingest`, Gmail sync) and OAuth callbacks.

## Components that matter most / least

Most:

- Authentication and sessions: `internal/identity/auth` (JWT cookie carrying `token_version`, API keys
  hashed with SHA-256, scopes), `internal/identity/auth/oauth`, `internal/identity/accounts`. That includes
  the seizure rule: an unverified, password-backed account is taken over by a provider-verified OAuth
  identity for the same address, and anything that links an account to an UNVERIFIED address is critical.
- Authorization: per-user data (`/me/...` routes, CVs, tracking, applications, the answer bank) leaking
  across accounts; the moderator role gate; API-key scope escapes (a `cv` key reaching `full` routes).
- SSRF in anything that fetches a URL taken from input: the crawl workers, `internal/ingest/linksource`,
  link import, the apply-form capture and auto-apply browser (`internal/api/atsapply`), the OG-image
  renderer. Outbound fetches are meant to go through `internal/platform/safehttp`.
- Injection reachable through the API or ingest: SQL (queries are sqlc-generated, so look for dynamic SQL
  built outside `internal/platform/db/queries`), Meilisearch filter injection through search parameters,
  stored XSS from crawled HTML rendered by `web/`.
- Billing and plan limits: anything that grants Pro/Ultra without a payment (`internal/identity/billing`,
  Stripe webhooks) or bypasses the per-day AI allowance (`internal/ai/plan`).

Less important but in scope: the cron workers' handling of crafted crawl data (panics and crashes,
unbounded memory), the browser extension under `extension/`.

Out of scope: see "Out of Scope" in SECURITY.md (self-hosting misconfiguration, plain volumetric DoS,
prompt injection of the enrichment LLM that crosses no real boundary, unreachable dependency bugs).

## How to exercise it

The image carries every service the server needs. All binaries are built into `/src/bin`.

```sh
# Postgres 18 + pgvector
mkdir -p /tmp/pg && chown postgres /tmp/pg
su postgres -c 'initdb -D /tmp/pg -U postgres --auth=trust && pg_ctl -D /tmp/pg -l /tmp/pg.log start'
psql -U postgres -c "CREATE USER hire PASSWORD 'hire' SUPERUSER" -c "CREATE DATABASE hire OWNER hire"
redis-server --daemonize yes
meilisearch --master-key masterKey1234567890 --db-path /tmp/meili --no-analytics &

export DATABASE_URL=postgres://hire:hire@localhost:5432/hire?sslmode=disable
export MEILI_URL=http://localhost:7700 MEILI_MASTER_KEY=masterKey1234567890
export JWT_SECRET=$(head -c 48 /dev/urandom | base64)
/src/bin/migrate -dir /src/migrations
/src/bin/server        # listens on :8080; the API lives under /api/v1
```

The LLM features stay off without `LLM_BASE_URL`/`LLM_API_KEY`/`LLM_MODEL`, which is the expected state
offline. `go test ./...` runs the unit tests. The `-tags=integration` tests need Docker (testcontainers)
and will not run here; read them for fixtures and for how handlers are wired (`internal/api/handler` holds
most of them). Crawl adapters live in `internal/ingest/sources` and come with recorded fixtures.

## How we rate severity

- **Critical**: account takeover, authentication bypass, reading or writing another user's data
  (CV, applications, mail, API keys), SSRF that reaches internal addresses or cloud metadata, SQL
  injection, remote code execution, leaking server secrets.
- **High**: privilege escalation to moderator, API-key scope escape, stored XSS that runs on freehire.me
  from crawled or user-supplied content, granting a paid plan or bypassing plan limits without paying.
- **Medium**: reflected XSS, CSRF on a state-changing route, open redirect, a crafted posting that crashes
  or hangs an ingest worker, DoS with real amplification.
- **Low**: information disclosure without user data, missing hardening.

## Anything to leave alone

- Wrong or junk enrichment output (the LLM's classification of a posting) is filtered by
  `Enrichment.Sanitize`/`Validate` and is not a vulnerability by itself.
- Inaccurate job content: postings are third-party data.
- Findings that need a deliberately weak configuration (`COOKIE_SECURE=false`, a short `JWT_SECRET`,
  an exposed Meilisearch master key).
