# Bound deep-offset pagination on the Postgres-backed lists

## Why

On 2026-09-14 the site answered 504 to 14,007 requests between 13:06 and 14:00 UTC.
Nothing was broken: one crawler walked `GET /api/v1/jobs?offset=` in steps of 100, eight
requests at a time, at ~600/min. By 13:06 its offset had reached ~160,000 and the site
stopped answering.

`ListJobs` is `SELECT * FROM jobs WHERE closed_at IS NULL AND duplicate_of IS NULL AND NOT
is_private ORDER BY created_at DESC, id DESC LIMIT $1 OFFSET $2`. `OFFSET k` reads and
discards `k` rows before returning any, and `SELECT *` plus the two predicates outside the
index mean every skipped tuple is a heap fetch against a table of millions of open rows. At
`offset=179500` one request walks ~180,000 of them.

The pool is 10 connections per process (`internal/platform/database/database.go`,
`defaultMaxConns`). Postgres showed all ten pinned in `ListJobs`, each for over two minutes,
every one waiting on `DataFileRead`/`AioIoCompletion` — disk, not locks: `pg_blocking_pids`
was empty and no lock was ungranted. With the pool exhausted every other endpoint queued
behind it. `GET /api/v1/threads`, whose own query is keyset-paginated and index-served, was
answered in **15m41s** — all of it spent waiting to acquire a connection, not running.

Three defences existed and none of them could bite:

1. **The per-IP rate limiter worked as designed and was irrelevant.** The crawler got 2,406
   × 429 and still took the site down. `publicReadsPerMinute = 600` bounds requests per
   minute; `public_read_limit.go` splits its budgets "by cost, not by path" — a split that
   assumes a path HAS a fixed cost. `/api/v1/jobs` does not: its cost is linear in an
   argument the caller chooses.
2. **`maxSearchWindow = 10000` already encoded the right rule** — but only on the two
   Meili-backed routes (`search.go`, `swipe.go`). Every Postgres-backed list was left
   unbounded, because their OFFSET walk was merely slow rather than refused.
3. **Fiber's `WriteTimeout: 10s` does not cancel the query.** It closes the socket; the
   handler goroutine and its in-flight pgx query keep running and keep holding the pooled
   connection. A client that gives up after 10s and re-requests stacks concurrent walks.

The outage ended by accident: the 13:59 autodeploy flipped nginx from blue to green, and the
fresh process had an empty pool. The wedged blue API was still not answering its own
`/health` an hour later.

## What changes

**One pagination window for every list endpoint, whichever store answers it.**

- `maxPageWindow = 10000` moves to `handler.go` and replaces `maxSearchWindow`. One number,
  not two that can drift.
- A new `pageParamsWindowed` returns `400 "pagination too deep"` when `offset+limit` reaches
  past it. It sits beside `pageParamsBounded` because `pagination_rule_test.go` already pins
  the rule that `handler.go` is the only file allowed to read the `offset` query param.
- Applied to the five Postgres-backed lists that had no bound: `GET /api/v1/jobs`,
  `GET /api/v1/companies`, `GET /api/v1/companies/:slug`, `GET /api/v1/jobs/:slug/copies`,
  `GET /api/v1/companies/:slug/feedback`. `search.go` and `swipe.go` move onto the shared
  helper rather than keeping their own copy of the check.

**Refuse, don't clamp.** A clamped page answers 200 carrying rows the caller did not ask
for. A walker loops over the same page forever and a person paging silently sees the wrong
slice. The repo's own convention — `meta.ignored_params` exists so a widened answer says so —
is that a silently different answer is the worse failure. 400 is the honest one, and the
Meili routes already answer exactly this.

**`GET /api/v1/companies/:slug/feedback` gains the public-read limiter.** It is the one
public list mounted with no limiter at all, and each request fires three queries.

**A `statement_timeout` on the API server's pool.** The backstop for the endpoint nobody
remembered: it bounds the query a connection may hold. Server-only — the cron workers share
`internal/platform/database` and some legitimately run for hours.

**The status page learns to see a saturated pool.** `currentSiteHealth` calls exactly one
method on the pool, `Ping`, which succeeds while every connection is busy. Adding
`pool.Stat()` is one in-memory read and turns the outage's actual shape into a `degraded`
verdict instead of "All systems operational".

## What does NOT change

- **The pool stays at 10 connections.** Raising it moves the wall without removing it: 30
  concurrent deep-offset walks saturate the same disk. What this change removes is the
  unbounded cost per request, which is the thing that made 10 too few.
- **No keyset cursor on `/api/v1/jobs`.** A real second pagination mode is a larger change
  with its own wire shape; the window is what stops the bleeding, and nothing in production
  pages past 1,600 today. Noted as the seam, not built.
- **nginx gains no new `limit_req` zone.** The app limiter already throttles these paths and
  did fire; a second copy of the same ceiling in a second place is a second answer to
  maintain. The defect was the unbounded cost, not the missing throttle.

- **No Cloudflare rule either**, though the edge is live and terraform-managed
  (`freehire-ops/infra/cloudflare`) and its rate-limit ruleset exempts `/api/` — which is
  genuinely where this crawler walked in. Three reasons not to close it there:
  the zone is on the Free plan, where a WAF custom rule cannot use the `matches` operator,
  so "offset is a number past 10,000" has no expression short of enumerating digit-count
  prefixes; the `/api/` exemption is deliberate, because API clients need rates the page
  routes do not; and after this change a refused request costs a 400 with no database work
  at all, so blocking it one hop earlier saves a little origin CPU and none of what actually
  hurt. Worth revisiting if the zone is ever upgraded — noted, not built.

  Separately: `freehire-ops/README.md` still says the Cloudflare zone is "NOT applied yet —
  the domain's NS still point at Namecheap". It is applied and has been since 2026-08-16;
  the nameservers are Cloudflare's and the origin answers behind `cf-ray`. That line is
  corrected in this change, because an ops document that is wrong about what is in front of
  production is worse than one that is silent.
- **The 504-invisibility on `/status` is not fixed.** A 504 is generated by nginx for a
  request that never completed in Fiber, so `observability.ErrorRate` structurally cannot
  count it. Seeing that needs nginx's own log, which is the `site-alert.sh` watchdog's job.
  The pool-saturation signal covers this outage's shape from inside.

## Impact

- Affected specs: `api-rate-limiting`, `api-documentation`
- Affected code: `internal/api/handler/{handler,jobs,companies,copies,company_feedback,search,swipe,status}.go`,
  `internal/platform/database/database.go`, `cmd/server/main.go`
- Wire change: five endpoints answer 400 where they answered 200. `docs/API.md` and
  `web/static/openapi.yaml` record the window.
- No migration, no reindex, no backfill.
