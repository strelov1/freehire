# internal/application/agentview

Records the job postings served to agents — the hosted MCP servers (`/mcp`,
`/mcp/account`), OJCP (`/ojcp/v1/*`, `/ojcp/mcp`) and `/api/v1/agent/jobs/search` — as
a JSON Lines log, and aggregates it into per-day unique views. It is the agent-side
twin of [viewlog](../viewlog/AGENTS.md): nginx's access log cannot see these, since
every MCP call is a POST to one URL and a search answer is a list.

- **Scope, not flags.** A Fiber middleware on each agent route calls `Attach` with
  `c.Context().SetUserValue`; fasthttp's `RequestCtx.Value` reads user values and the
  adaptor hands that context to the MCP SDK, so `Record` finds the scope inside a tool.
  `Record` without a scope does nothing — that is what keeps the in-app assistant,
  which shares `search_jobs`/`get_job` with `/mcp/account`, out of the log.
- **Never log what nginx already counts.** `GET /api/v1/jobs/<slug>` is in `uniques`
  already; recording it here would count one read twice.
- **One file per UTC day** (`agent-views-YYYY-MM-DD.jsonl`), named by each event's own
  timestamp. No logrotate; the rollup reads only finished days, and only `SettleAfter`
  past their end — the writer drops an event later than `lateWriteCutoff` for its day, so
  a file the rollup claimed never changes afterwards.
- **Two kinds, one count.** `kind` (`search`/`detail`) stays in the log for analysis;
  `Aggregate` dedups on `(viewer, job, day)` alone.
- **Dropping beats blocking.** A full buffer drops the event and counts it.
- `cmd/rollup-views` applies the counts to `job_daily_views.agent_uniques` and
  `jobs.view_count`; `uniques` and `page_uniques` never move from here.
