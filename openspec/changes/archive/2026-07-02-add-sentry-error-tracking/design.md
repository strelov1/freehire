## Context

Errors today land only in per-process stdout (`log.Printf`, Fiber's `recover.New()`,
`RenderError`) and the browser console. There is no aggregation, so a panic in a cron
worker or a 500 in a handler is invisible unless someone reads logs. We want one Sentry
inbox fed by three surfaces: the Fiber HTTP server (`cmd/server`), the eleven
run-once-and-exit cron workers (`cmd/*` that call `internal/worker.Bootstrap`), and the
SvelteKit frontend (`web/`).

Constraints from the codebase:

- Optional integrations follow a consistent pattern: an env var gates them, and their
  absence causes no regression (Meili/LLM/S3/Telegram in `internal/config`).
- Config is centralized in `internal/config` (`Settings` + `Load`); the server reads it
  in `cmd/server/main.go`, workers via `worker.Bootstrap` → `config.Load`.
- All eleven cron workers share `internal/worker.Bootstrap` for config+pool+signal
  context — a natural single seam. The `harvest-*` and `gen-contracts` commands are local
  dev tools (no Bootstrap, not deployed) and are out of scope.
- The Fiber app already has `recover.New()` and a central `handler.RenderError`.
- The frontend is SvelteKit with `adapter-node` (SSR) and a strict CSP in
  `web/svelte.config.js` that sets `script-src`/`base-uri`/`object-src` but **no**
  `default-src` and **no** `connect-src`.

## Goals / Non-Goals

**Goals:**

- One env-gated Sentry init reused by the server and every cron worker.
- Capture unhandled panics and unexpected 5xx on the backend; capture panics and
  reported errors in workers with guaranteed flush before exit.
- Capture client and SSR errors in the frontend.
- No regression when unconfigured; no PII by default.

**Non-Goals:**

- Performance tracing / spans (sample rate 0). Can be a later change.
- Session replay.
- Structured breadcrumb/user-context enrichment beyond environment + process tags.
- Source-map upload pipeline is optional and ops-driven (documented, not required for the
  errors-only goal to function).
- Instrumenting the local `harvest-*` / `gen-contracts` dev tools.

## Decisions

### D1 — One backend helper package `internal/observability`

`observability.Init(Settings) (flush func())` wraps `sentry.Init` with the project's
defaults: DSN + environment from config, `EnableTracing:false`, `SendDefaultPII:false`,
a bounded `flush` (e.g. `sentry.Flush(2*time.Second)`). When the DSN is empty it returns
a no-op `flush` and does not initialize — mirroring how `search.NewClient`/`blobstore.New`
stay nil. Rationale: the server and all workers need identical init; a single helper keeps
defaults (PII off, environment tag) in one place. Alternative — inline `sentry.Init` at
each entry point — was rejected as duplicative and drift-prone.

Config gains `SentryDSN` (`SENTRY_DSN`) and `SentryEnvironment` (`SENTRY_ENVIRONMENT`,
default `"development"`) in `internal/config`, read like the other optional integrations.

### D2 — Workers: init in `worker.Bootstrap`, flush in its `cleanup`, panic capture via a thin `main` wrapper

`worker.Bootstrap` already returns `cleanup`. It will call `observability.Init` and fold
the returned `flush` into `cleanup` (which workers already `defer`), so **normal-exit
delivery is centralized in one file** — no per-worker flush edits. For panic capture we
add `worker.Main(run func() int)`:

```go
func Main(run func() int) {
    defer func() {
        if r := recover(); r != nil {
            sentry.CurrentHub().Recover(r)
            sentry.Flush(2 * time.Second)
            panic(r) // preserve the crash + non-zero exit
        }
    }()
    os.Exit(run())
}
```

Each worker's `func main() { os.Exit(run()) }` becomes `func main() { worker.Main(run) }`
— one mechanical line per worker. `os.Exit` on the normal path is unchanged (Sentry was
flushed by `cleanup` before `run` returned); on panic, `os.Exit` is never reached, the
deferred recover captures + flushes, then re-panics to keep the existing crash semantics.
Sentry is initialized inside `Bootstrap` (first call in `run`), so any panic after
bootstrap is captured; a panic before it (e.g. bad config) is not — acceptable.

Alternative — recover inside `cleanup` — rejected: `cleanup`'s job is resource teardown,
and swallowing/re-raising panics there muddies that responsibility (and risks changing
exit codes).

### D3 — Server: fibersentry middleware + capture in `RenderError`

Wire `github.com/getsentry/sentry-go/fiber` (fibersentry) early with `Repanic:true` so
the existing `recover.New()` still produces the standard 500 envelope while fibersentry
captures the panic on a request-scoped hub. For **non-panic** unexpected errors,
`handler.RenderError` reports to the request hub only on the fall-through 500 branch —
`*fiber.Error` (4xx), `pgx.ErrNoRows`→404, and FK-violation→404 are explicitly **not**
reported, keeping the inbox free of routine 4xx. `RenderError` gains the Fiber ctx it
already has to reach `fibersentry.GetHubFromContext(c)`; when Sentry is disabled the hub
is absent and reporting is skipped. The server calls `observability.Init` at startup and
its `flush` after `app.ShutdownWithTimeout`, next to the existing `tracerShutdown()`.

### D4 — Frontend: `@sentry/sveltekit`, gated on `PUBLIC_SENTRY_DSN`

Create `web/src/hooks.client.ts` and `web/src/hooks.server.ts` (neither exists yet):

- Both call `Sentry.init({ dsn: PUBLIC_SENTRY_DSN, environment, tracesSampleRate: 0 })`
  **only when the DSN is set**, and export `handleError = handleErrorWithSentry(...)`.
- `hooks.server.ts` also composes `sentryHandle()` into the `handle` sequence.
- The DSN is exposed as a public env var (`PUBLIC_SENTRY_DSN`) so the client bundle can
  read it; SvelteKit's `$env/static/public` requires the `PUBLIC_` prefix.

The Vite `sentrySvelteKit()` plugin (source-map upload) is added but is inert without
`SENTRY_AUTH_TOKEN` at build time — kept out of the required path; ops can enable it later.

### D5 — CSP: no live change; document the ingest host

The current CSP defines no `default-src` and no `connect-src`, so browser `fetch`/
`sendBeacon` to `*.ingest.sentry.io` is already unrestricted — the errors-only client SDK
loads from the same-origin bundle and injects no external script. Adding a `connect-src`
now would *tighten* the policy and risk breaking GA and same-origin API calls. Decision:
**do not add a live `connect-src`**; instead add a comment in `web/svelte.config.js`
recording the Sentry ingest host so that if a future change introduces `connect-src`, the
host is included. This satisfies the spec's "not blocked by CSP" requirement with zero
regression risk.

## Risks / Trade-offs

- **[Minified frontend stack traces without source maps]** → Errors still group and report;
  source-map upload is documented as an optional ops follow-up (D4). Acceptable for v1.
- **[Worker panic before `Bootstrap` isn't captured]** → Only config/DSN-load failures fall
  in this window; they already exit non-zero and log. Acceptable.
- **[PII leakage]** → `SendDefaultPII:false` on every surface (backend + `handleErrorWithSentry`
  defaults) keeps cookies/auth/bodies/emails off events; enforced by the spec requirement.
- **[Double-reporting a server panic]** (fibersentry + a future RenderError catch) →
  RenderError reports only the non-panic 500 branch; panics never reach it because
  `recover.New()` handles them. No overlap.
- **[Adding a new dependency to 11 worker binaries]** → `sentry-go` is small and pure-Go;
  no cgo. Build/image size impact negligible.

## Migration Plan

1. Land code behind env gates (no DSN in any current env ⇒ dormant everywhere).
2. Create two sentry.io projects (frontend + backend); obtain two DSNs.
3. In `freehire-ops`, inject `SENTRY_DSN`/`SENTRY_ENVIRONMENT` into server + worker
   containers and `PUBLIC_SENTRY_DSN`/`PUBLIC_SENTRY_ENVIRONMENT` into the web build/runtime.
4. Deploy; verify a test error appears in each project.

- **Rollback:** unset the DSNs — the integration goes fully dormant with no redeploy of code
  required (though clearing build-time `PUBLIC_SENTRY_DSN` needs a web rebuild).

## Open Questions

- Do we want `SENTRY_RELEASE` (git SHA) tagged now for regression tracking, or defer until
  the source-map pipeline lands? (Leaning defer — not needed for errors-only v1.)
