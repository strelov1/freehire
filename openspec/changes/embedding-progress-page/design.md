## Context

Semantic embeddings live in `job_semantic_chunks` and are produced by `cmd/embed`
(server-side TEI) and by volunteers through `freehire-volunteer-embed`. Both drain
`semantic_outbox`, which only `cmd/embed` fills, at start, through
`EnqueuePendingSemanticJobs`; `freehire-embed.timer` (hourly, `freehire-ops`) now runs
it on a schedule.

Two measurements were taken on production on 2026-10-19 that shape everything below:

- Counting eligible and covered postings (the enqueue predicate over ~6M open rows)
  did not finish inside a 60-second statement timeout on a loaded host.
- Counting claimable `semantic_outbox` rows took 4.5 ms (partial index
  `semantic_outbox_claimable_idx`).

Existing patterns this follows: `internal/ingest/catalogstats` (a snapshot measured in
`cmd/rollup-stats`, stored in the cache, read by handlers that never count), and
`internal/job/recentfeed` (an in-process poller in `cmd/server` feeding an in-memory
broadcaster behind an SSE route, `recent_feed_stream.go`).

## Goals / Non-Goals

**Goals:**

- A public, honest figure for how much of the catalogue is embedded and how fast the
  rest is going, updated live.
- A route a volunteer can watch move while their client runs.

**Non-Goals:**

- Per-volunteer statistics or a leaderboard (the broker owns that data; a later change
  could read it).
- Alerting — `cmd/queue-metrics` already exports the outbox depth to Prometheus.
- Exact coverage between snapshots.

## Decisions

### 1. Two sources, one document

Coverage is expensive and slow-moving; the queue is cheap and fast-moving. Measure each
where its cost belongs and combine them at read time:

- **Snapshot** (`eligible`, `covered`, `snapshot_at`) — a pass in `cmd/rollup-stats`,
  stored under one cache key with a TTL long enough to survive a skipped run, exactly
  as `catalogstats` does. Run with its own `statement_timeout` so a slow night cannot
  hold the rollup worker open.
- **Live** (`queued`, `measured_at`, rate) — the sampler in `cmd/server`.
- **Combined**: `covered = clamp(eligible − queued, 0, eligible)`. The queue is, by
  construction, the set of eligible postings that are not covered (the enqueue
  predicate is the coverage predicate), so this is current to the last hourly enqueue
  rather than to the last snapshot. It overcounts the uncovered slightly — the queue
  also carries closed postings waiting to have their state cleared — which errs on the
  side of under-claiming progress.

Alternative rejected: count coverage per request with a cache in front. Even one
cold request is a minute-long scan on the host whose bottleneck is the database.

### 2. The sampler lives in `cmd/server`, not in `cmd/queue-metrics`

`cmd/queue-metrics` already reads the depth every minute, but it is a separate process
that publishes to the Prometheus textfile collector; handing its samples to the API
would add a cache write and read per minute and still leave the stream needing its own
tick. A 4.5 ms query every 15 seconds per API instance costs nothing, and keeping the
ring in memory matches `recentfeed`. A restart or blue/green flip loses the ring: the
page says "measuring…" for the rate for five minutes — the same accepted gap as the
recent-jobs feed.

The depth query is new and narrow (`count(*) WHERE failed_at IS NULL`) rather than
`SemanticOutboxMetrics`, which also scans for dead letters and the oldest row.

### 3. Rate counts only decreases

The hourly enqueue adds a whole hour of postings in one step. A rate taken as
(first − last) / time would go negative after every enqueue and the ETA would vanish
every hour. Summing only the decreases between consecutive samples measures what was
drained, which is what "speed" means to a reader. Window: 10 minutes (40 samples);
no rate until the ring spans 5 minutes.

### 4. Package placement: `internal/search/embedprogress`

The coverage predicate needs `search.CurrentEmbedderModel()`, which lives in the
`search` block (layer 6); `ai` (layer 3), where `embed` lives, cannot import it. The
package holds the pure parts (the snapshot type and its cache key, the sample ring and
rate maths, the combined document) plus the sampler and broadcaster. It is added to
`internal/platform/arch/layering/blocks.go`.

### 5. Stream shape follows `recent_feed_stream.go`

Same `sseHeaders`/`newSSEStream`/keepalive helpers, same per-IP connect limiter, an
event named `progress` carrying the full document (not a delta — a client that
missed one event is current again at the next). On connect the current document is
sent first, so a new client never waits 15 seconds for its first figure.

### 6. The page

`web/src/routes/embeddings/+page.server.ts` loads the JSON endpoint (best-effort, like
`/status`: a failed read renders the page with every figure "measuring…"), then
`+page.svelte` opens an `EventSource` on mount and replaces the document on each
event. The volunteer block links the repository and `HOW_TO_RUN_THE_CLIENT.md`. Built
from design-system primitives; a null figure renders "measuring…".

## Risks / Trade-offs

- [The coverage pass adds a minute-plus scan to `cmd/rollup-stats` every 3 hours] →
  bounded by its own `statement_timeout`, best-effort, after the rollups commit.
- [`covered` is approximate between enqueues] → stated on the page next to the figure.
- [If `freehire-embed.timer` stops, the queue empties and the live figure reads 100%
  while new postings go unembedded — the failure found on 2026-10-19] → the document
  also carries the snapshot's own measured count (`snapshot_covered`), and the page
  shows it beside the live one with its time. A live 100% that a recent measurement
  contradicts is then visible to anyone looking, not only to an operator.
- [Many open streams] → per-IP connect limit; each subscriber is one channel on an
  in-memory broadcaster, as with the recent-jobs feed.

## Migration Plan

No schema change. Deploy order: release (`release.sh`) ships the server and the
worker together; the page shows "measuring…" for eligible/covered until the next
`cmd/rollup-stats` run (or a manual one). Rollback: revert; nothing persists beyond
one cache key.

## Open Questions

None.
