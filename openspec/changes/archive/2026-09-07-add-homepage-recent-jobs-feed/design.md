## Context

`cmd/ingest` (writes jobs) and `cmd/server` (serves the homepage) are separate
OS processes: `cmd/ingest` is a run-once-and-exit cron worker, never
long-lived, so nothing it does can push directly into an already-open SSE
connection held by `cmd/server`. `cmd/server` itself runs as a single active
instance at a time (blue/green deploy behind nginx, not N load-balanced
replicas — see `deploy/AGENTS.md`), which simplifies fan-out but does not
remove the cross-process problem.

The pipeline already has a proven pattern for exactly this shape of problem:
`search_outbox`, written in the same transaction as `UpsertJob` and drained by
a separate process on its own cadence (`internal/search/searchdrain`). This
design reuses that pattern rather than introducing a new transport.

The `jobs` table already carries `role_fingerprint`, computed at write time —
but it hashes `company_slug` together with the normalized title and
description (`internal/job/jobhash.RoleFingerprint`), so it identifies one
company's own reposts of a role across cities, not the same role posted by
*different* companies. It is the wrong key for this feed's aggregation (see
Decisions); the normalization helper it already applies to a title is the
part this design reuses. See proposal.md - Why for the product motivation.

## Goals / Non-Goals

**Goals:**
- Bridge "a job was ingested" from `cmd/ingest` to open SSE connections on
  `cmd/server` using infrastructure the project already runs (Postgres), with
  a-few-seconds latency being acceptable.
- Aggregate bursts of same-role postings into one entry, reusing existing
  clustering data instead of new title-normalization logic.
- Keep the feed's blast radius small: unauthenticated, read-only, cosmetic —
  never a dependency other features rely on for correctness.

**Non-Goals:**
- Sub-second delivery. This is a "the catalogue is alive" signal, not a
  monitoring or alerting feed.
- Durability across restarts. The ring buffer and any in-flight grouping
  state may be dropped on deploy without any user-facing correctness issue —
  only a brief, cosmetically empty feed on the freshly-flipped instance.
- Covering every ingest source. `cmd/tg-extract` stays out of scope, matching
  its existing exclusion from `search_outbox`.

## Decisions

### Poll-based outbox instead of Postgres LISTEN/NOTIFY or Redis pub/sub
Reuses the existing `search_outbox` shape (table + periodic drain) instead of
introducing a new transport. Alternatives considered:
- **Postgres LISTEN/NOTIFY**: near-instant, but needs a dedicated long-lived
  listen connection with its own reconnect handling, and buys precision the
  product doesn't need (aggregation already implies waiting for a window to
  fill). Rejected as unnecessary complexity for a cosmetic feed.
- **Redis pub/sub**: `cmd/server` already holds a Redis client, but
  `cmd/ingest` does not, and wiring a new dependency into a cron worker for
  a single non-critical signal is a bigger footprint than one more Postgres
  table. Rejected in favor of the pattern already proven at this write site.

### Poller runs inside `cmd/server`, not as a separate `cmd/*-drain` binary
Every existing outbox drain (`cmd/search-drain`, enrichment) is its own
run-once-and-exit binary on a systemd timer. This feed instead needs its
drain loop to live inside the same long-lived process that holds the SSE
connections, since the whole point is pushing into those connections
directly — a separate process draining the outbox would still need a second
hop to reach `cmd/server`. The poller is a ticker goroutine started at
`cmd/server` boot, not a new binary under `cmd/`.

### Grouping key: normalized title text, not `role_fingerprint` or raw title
`role_fingerprint` was the first candidate — it is already computed and
stored on every job — but it hashes `company_slug` into the same digest
(`internal/job/jobhash.RoleFingerprint`), so it clusters one company's own
reposts of a role, never the same role posted by different companies. It
cannot serve this feed's aggregation: grouping by it would put every
company's postings in their own singleton group, defeating the point.

Instead, `internal/job/jobhash` gains one new exported function,
`NormalizedRoleTitle(title string) string`, extracted from the existing
unexported `normalizeRoleText`/`stripTrailingClause` logic `RoleFingerprint`
already applies to a job's title (tag-stripped, entity-decoded, lowercased,
whitespace-collapsed, trailing clause removed) — minus the company slug and
description that made the combined hash company-scoped. `recentfeed` groups
a claimed batch by this normalized title. This is still "reuse an existing,
proven normalization" rather than new text-matching logic, just at the
right scope: the part of `RoleFingerprint` that already understands title
variation, without the part that scopes it to one company.

Raw exact-title matching was also considered and rejected on its own
merits: it would under-group (e.g. "Senior Go Engineer" vs "Senior Golang
Developer" treated as different roles), producing exactly the flood of
individual cards the aggregation requirement exists to prevent — the same
reason `NormalizedRoleTitle` reuses `RoleFingerprint`'s text normalization
rather than a plain string comparison.

### Aggregation threshold is a code constant, not an env var
Unlike the repo's `BACKFILL_*`/`*_MAX_PER_RUN` operational knobs, this
threshold is product behavior (what a visitor sees), not an operational
safety valve for a one-off run. It belongs in code review, not prod config.

### In-process ring buffer + fan-out, no persistence
Since `cmd/server` runs as a single active instance, an in-process broadcaster
is sufficient for the common case. The accepted gap: a freshly-flipped
blue/green instance starts with an empty ring buffer until the first poll
tick (~10s) repopulates it — purely cosmetic, no data is lost (the outbox
rows themselves are durable in Postgres until claimed).

### Aggregated entries do not attribute a single company
An aggregated entry represents postings from potentially many different
companies. The design explicitly requires the UI not to present the one
sample logo/company as if all N postings came from it — the entry names the
role and a count, with the shown company clearly one representative example.

### Logos: reuse `EntityLogo` + `companyLogoUrl`, no `logo_url` on the wire
`companies` has no `logo_url` column at all — `JobRow.svelte` already
resolves a company's mark client-side via `companyLogoUrl(name)`
(`web/src/lib/logo.ts`, a proxy keyed by company name) rendered through the
design-system `EntityLogo` primitive, which already falls back to a monogram
on a broken/missing image. The new feed reuses both exactly as `JobRow`
does: the backend event payload carries only the company name (and job/
company slugs for linking), never a logo URL, and the frontend never
implements its own placeholder logic.

## Risks / Trade-offs

- **[Risk]** Outbox rows for eligible jobs pile up if the poller stalls
  (e.g. a bug in the grouping step). → Mitigation: claim-and-delete is a
  bounded batch per tick (a cap, not unlimited), so a stalled poller degrades
  to "feed is behind" rather than an unbounded table or query.
- **[Risk]** A very bursty ingest run (e.g. `INGEST_REFETCH_ALL=1`, see root
  AGENTS.md) could enqueue a very large number of rows at once. → Mitigation:
  the per-tick batch cap plus role-based aggregation means a large burst
  collapses into a handful of aggregated entries rather than flooding the
  feed or the outbox table.
- **[Trade-off]** No cross-instance delivery guarantee during a blue/green
  flip. Accepted per Goals/Non-Goals — this is a cosmetic feed, not a
  durability-sensitive one.
- **[Risk]** Homepage becomes a public, unauthenticated, always-open
  long-lived connection endpoint — a new minor surface for connection
  exhaustion. → Mitigation: reuses the existing SSE keepalive/write-timeout
  machinery already hardened for the assistant and match-analysis streams
  (`internal/api/handler/match_analysis_stream.go`), which already handles
  this class of concern; no new safeguards invented here.

## Migration Plan

1. Add the `recent_feed_outbox` migration (additive only — no existing table
   or column changes).
2. Ship the `cmd/ingest` enqueue call, the poller/broadcaster package, and the
   SSE endpoint together, since the endpoint is inert (empty ring buffer)
   until the enqueue path is live — order between them does not matter for
   correctness.
3. Ship the frontend component last, once the endpoint is verified manually
   (`make run`, confirm events arrive) — the homepage renders nothing extra
   until the component is added, so this is a safe, independently revertible
   final step.
4. Rollback is a plain revert: dropping the migration, the enqueue call, or
   the frontend component independently leaves the rest of the system
   correct (outbox rows left unclaimed are harmless and never read by
   anything else).
