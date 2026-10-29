# Agent view counts

## Problem

A posting's view counter (`jobs.view_count`, `job_daily_views`) is built by
`cmd/rollup-views` from nginx access logs (`internal/application/viewlog`). It
counts a 2xx `GET` of `/jobs/<slug>` (page) and `/api/v1/jobs/<slug>` (API). Every
agent surface that serves jobs some other way is invisible to it:

- the hosted MCP servers (`/mcp`, `/mcp/account`) and OJCP (`/ojcp/mcp`) — every
  call is a `POST` to one URL, so the log line says nothing about which jobs came
  back;
- search results on any surface — a list is never a view, so a posting an agent
  found and read inside a search answer counts nothing.

## Decisions

- **Two event kinds, one count.** The log records whether a posting was returned in
  a `search` result or as a `detail`. The rollup ignores the kind: both are one
  agent view. The kind stays in the log so a later analysis can split them.
- **Agent views add to `jobs.view_count`**, the number the job page shows. They are
  also kept apart in a new column, `job_daily_views.agent_uniques`, because once
  summed into `view_count` nothing else could tell them from people.
- **`page_uniques` is untouched.** It stays the only human-only figure, and the
  social digest keeps ranking on it.
- **Nothing nginx already counts is logged again.** `GET /api/v1/jobs/<slug>` (the
  CLI's and the local `freehire-mcp` package's detail read) is already in
  `uniques`; logging it here would count one read twice. The SPA's search
  (`/api/v1/jobs/search`) and list (`/api/v1/jobs`) are human traffic and are not
  logged either.

## What is logged

One event per served response, from these call sites:

| Surface (`surface`) | Search (`kind=search`) | Detail (`kind=detail`) |
|---|---|---|
| `mcp` — `/mcp` (`internal/api/handler/mcpapp.go`) | `search_jobs`, `similar_jobs` | `get_job` |
| `mcp_account` — `/mcp/account` (`assistant_tools.go`) | `search_jobs` | `get_job` |
| `ojcp` — `/ojcp/v1/*` and `/ojcp/mcp` (`ojcp.go`) | `search` | `get_job_detail` |
| `agent_api` — `/api/v1/agent/jobs/search` (`agent_search.go`) | yes | — |

`check_jobs_open`, `my_jobs` and `subscription_updates` are not logged: the first
returns status only, the other two return the caller's own saved state.

The `/api/v1/agent/jobs/search` hook sits in `agentSearch`, not in the shared
`runJobSearch`, which also serves the SPA.

## Log format

JSON Lines, one object per event:

```json
{"ts":"2026-10-28T14:03:11Z","kind":"search","surface":"mcp","viewer":"ip:203.0.113.7|ua:Claude-User","client":"claude-ai","job_ids":[123,456]}
```

- `viewer` is the dedup identity: `user:<id>` when the caller is signed in
  (`/mcp/account`, a key on `agent_api`), otherwise the client IP (`c.IP()`, which
  already resolves `X-Real-IP` from the trusted proxy) plus User-Agent, the same
  pair the nginx rollup dedups on.
- `client` is the MCP `clientInfo.name` when the call carries one, else empty. It
  is for analysis only and is not part of the dedup key.
- Job ids, not slugs: every call site already holds the id, and the rollup then
  needs no slug lookup.

## Writer

A small package, `internal/application/agentview`, owns both ends: the event type,
the writer, and the parser/aggregator (semantics live in one place, as in
`viewlog`).

- `AGENT_VIEW_LOG_DIR` names the directory. Unset, the writer is a no-op — tests
  and local runs need nothing, and that is also the rollback.
- The writer appends to one file per UTC day, `agent-views-YYYY-MM-DD.jsonl`, named
  by the event's own timestamp. No logrotate: a day's file is complete once the day
  is over, so the rollup never reads a half-written day and never sees the
  after-midnight stub nginx's rotation leaves behind.
- Off the request path: handlers send to a buffered channel and one goroutine
  writes. When the channel is full the event is dropped and a counter is
  incremented, never blocking a response — losing a view costs a number, blocking
  costs a request.
- Both blue/green server instances may write the same file. Each event is one
  `write(2)` of one line to an `O_APPEND` file, which Linux appends atomically for
  regular files, so lines never interleave.

## Rollup

`cmd/rollup-views` gains a second pass over `AGENT_VIEW_LOG_DIR`:

- It reads every `agent-views-*.jsonl` whose day is before today (UTC), skipping
  files already in `processed_view_logs` (same content-hash cursor the nginx pass
  uses, so the two never collide in practice and a re-run applies nothing twice).
- It dedups by `(viewer, job_id, day)`, ignoring `kind` and `surface`.
- It writes through a new query beside `ApplyDailyView`: upsert
  `job_daily_views.agent_uniques += delta` and `jobs.view_count += delta`, file
  counts and the processed mark in one transaction, ordered by job id with the
  same deadlock retry.
- After a successful pass it deletes processed files older than 14 days, the
  retention nginx's logs already have.
- An unparseable line is skipped and counted; a file that is entirely unparseable
  fails the run, since a day that parses as nothing would silently count nothing
  (the trap `viewlog/AGENTS.md` names for the nginx format).

## Schema

One migration: `ALTER TABLE job_daily_views ADD COLUMN agent_uniques int NOT NULL
DEFAULT 0`. `uniques` is not touched, so `GET /api/v1/stats/catalog` keeps its
current meaning.

## Known limit

A scraper paging through the whole catalogue over MCP adds +1 per posting per day.
The per-day dedup caps it at one, but an agent and a scraper cannot be told apart
the way page-open bots can, so nothing filters it. `agent_uniques` keeps that
traffic measurable on its own.

## Deploy

1. Ship the code with `AGENT_VIEW_LOG_DIR` unset — no behaviour change.
2. Create the directory on the host (writable by the server's user, readable by
   `rollup-views`), set the variable for both the server and `rollup-views` in
   `freehire-ops`.
3. The next day, check that `job_daily_views.agent_uniques` has values for
   yesterday.

## Tests

- Writer: one line per event, the file chosen by the event's UTC day, a full
  channel drops instead of blocking, an unset directory writes nothing.
- Parser/aggregator: one viewer seeing a job in a search and then as a detail on
  the same day is one view; two days are two; two viewers are two.
- Rollup: `agent_uniques` and `view_count` move by the same delta, `uniques` and
  `page_uniques` do not; a processed file is not applied twice; today's file is not
  read.
- Call sites: each logged surface emits the right `kind` and ids; the REST detail
  route and the SPA search emit nothing.
