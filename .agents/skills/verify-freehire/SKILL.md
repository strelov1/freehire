---
name: verify-freehire
description: Launch an isolated freehire stack from this checkout (Postgres DB, Meilisearch, the Go API, the SvelteKit web app, one real board ingested) and drive it like a user with Playwright to prove a change works. Use before calling a change done, when checking a pull request is ready to merge, or when asked to run, see or screenshot freehire locally. Works on the laptop (mise + Docker) and in the SuperPlane coder runner.
---

# verify-freehire

Proves a change in the real app: the API built from THIS checkout, the web dev server in
front of it, real postings crawled from one live board, a real browser. Tests passing is
not this; this is the user path.

Everything lives in this folder. Run commands from the repository root.

| Helper | What it does |
|---|---|
| `.agents/skills/verify-freehire/stack.sh up` | start everything, seed, wait until ready, print `doctor` and the env |
| `.agents/skills/verify-freehire/stack.sh doctor` | read-only health check of the running instance |
| `.agents/skills/verify-freehire/stack.sh env` | the exports a drive needs: `eval "$(.agents/skills/verify-freehire/stack.sh env)"` |
| `.agents/skills/verify-freehire/run-journey.sh <spec>` | run one Playwright journey against the instance, evidence included |
| `.agents/skills/verify-freehire/stack.sh down` | stop what `up` started, drop its database; evidence stays |

## Launch

```bash
.agents/skills/verify-freehire/stack.sh up      # ~1 min warm, longer on a first pnpm/go build
```

What `up` does, in order:

1. **Postgres.** Uses `TESTDB_ADMIN_URL` (mise sets it on the laptop; the coder runner's
   image sets it and starts Postgres itself). On the laptop, if nothing answers, it runs
   `docker compose up -d db`. Creates database `verify_<api port>` and runs `cmd/migrate`.
2. **Meilisearch**, private to this instance on `api port + 2`: a `meilisearch` binary on
   PATH if there is one, else a container `verify-freehire-meili-<port>` from the image
   `docker-compose.yml` pins. Neither: it warns and `/jobs` will answer 500.
3. **Seed.** `cmd/reindex` (creates the empty index), `cmd/add-board greenhouse/vercel`,
   `cmd/ingest greenhouse` (crawls the live board, ~90 postings, needs internet),
   `cmd/search-drain` (pushes them into Meilisearch). Override with
   `VERIFY_SEED_PROVIDER` / `VERIFY_SEED_BOARD`; `VERIFY_SEED_BOARD=` skips seeding.
4. **API**: `go build ./cmd/server`, started on the api port with `JWT_SECRET`,
   `FRONTEND_ORIGIN` and `MEILI_*` set. Ready = `/health` says `"database":"up"`.
5. **Web**: `pnpm install` in `design-system/` and `web/`, then `vite dev` on `api port + 1`,
   proxying `/api` to the API. Ready = `GET /jobs` answers 200 (the first SSR compiles).

Ports: the api port is `$VERIFY_API_PORT`, else mise's per-worktree `$PORT`, else 18080.
State (pids, logs, the server binary) is in `/tmp/verify-freehire-<api port>`.
Logs to read when something is off: `server.log`, `web.log`, `seed.log`, `migrate.log` there.

Not running, on purpose: Redis (the rate limiter fails open, the log shows `connection
refused`, harmless), mail (no SES: sign-up works, email verification codes never arrive),
OAuth providers, payments, LLM features (no `LLM_*`: AI routes refuse).

**Isolation.** Two instances can run side by side if their api ports differ (each worktree
gets its own `$PORT` from mise). `up` refuses to start over a live instance in the same
state dir. Do not point a drive at someone else's instance.

## Doctor

```bash
.agents/skills/verify-freehire/stack.sh doctor
```

Every line must say `ok`. It checks: the server was built from the current `HEAD` (a stale
binary proves nothing about your change: `down` + `up` after committing), the API and web
processes are the ones `up` started, `/health` sees the database, Meilisearch is healthy,
search holds seeded postings, `/jobs` renders. Run it first whenever anything looks off.

## Drive

Playwright from `web/node_modules` with its pinned Chromium (the runner has it in
`/opt/ms-playwright`; on a laptop without it: `cd web && pnpm exec playwright install chromium`).

```bash
.agents/skills/verify-freehire/run-journey.sh .agents/skills/verify-freehire/journeys/jobs-search.spec.ts
```

The journeys in `journeys/` are the mapped features (see `features/README.md`). For a
change no journey covers, write a spec of your own the same way (any path; the helper
copies it into `web/e2e/__verify__/` for the run and deletes it after) and run it with the
same helper. If it is a real regression test, also add it under `web/e2e/` in the PR.

Rules that came from failed runs:

- **Wait for hydration.** The page is server-rendered; typing or submitting before Svelte
  hydrates is lost (a form submits as a native GET). `await page.waitForLoadState('networkidle')`
  after every `goto` that is followed by input.
- **Cold routes are slow.** `vite dev` compiles each route on its first visit (5-15s), so a
  journey uses `expect.configure({ timeout: 30_000 })` and `test.setTimeout(120_000)`, as
  the shipped ones do. `up` warms `/jobs` and one job page.
- **Stable handles**: `getByTestId('job-card')`, roles with names (`combobox 'Search jobs'`,
  `button 'Create your account'`), `getByLabel('Email')`. When a locator misses, read the
  page snapshot in the failure's `error-context.md`; it lists every role and name.
- The feed may open a posting in a new tab (`target=_blank`): follow `context.waitForEvent('page')`.
- API checks go through the web origin (`page.request.get('/api/v1/...')`) or straight to
  `$VERIFY_API_URL` with curl.
- Database side effects: `psql "$DATABASE_URL"` after `eval "$(stack.sh env)"`.

## Evidence

Everything goes to `$VERIFY_EVIDENCE` (default `/tmp/verify-freehire-evidence`), which
`down` never touches:

- `<journey>-<n>-<step>.png`: screenshots the journey takes at each step (action, then result);
- `<journey>.log`: the Playwright run;
- `<journey>-playwright/`: the trace (`pnpm exec playwright show-trace <trace.zip>`) and,
  on failure, `error-context.md` with the page snapshot;
- anything you add: save `psql` / `curl` output there too (`| tee "$VERIFY_EVIDENCE/x.txt"`).

Proof standards:

- Drive the real user path: the UI or the public API. No internal setters, no test-only
  routes, no SQL writes to fake a state the feature should have produced.
- Capture the action and the resulting state, not only the last screen.
- Check side effects next to what is visible: the row in Postgres, the document in search.
- A journey that fails because the seed lacks something is a failure; write the reason in
  that feature's Gotchas. Never weaken the journey to make it pass.
- In a pull request, name what you drove, link or attach the screenshots, and say what you
  could not drive (mail, payments, LLM) instead of implying it was covered.

## Cleanup

```bash
.agents/skills/verify-freehire/stack.sh down
```

Stops the processes `up` recorded (by pid, never by name), removes its Meilisearch
container, drops `verify_<port>`, deletes the state dir. It leaves the shared compose
Postgres running and the evidence folder in place. Run it after every failed attempt too.
