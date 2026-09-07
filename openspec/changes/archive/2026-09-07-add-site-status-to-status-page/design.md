## Context

See `proposal.md` - Why. Constraints already settled with the user before
this change was formalized:

- No live query against the production Prometheus and no change to the
  separate `freehire-ops` repo (which owns the actual Prometheus/Grafana/
  alert-rule config) — a cross-repo dependency for a status-page nicety was
  rejected.
- No new database table or cron worker — the existing per-provider ingest
  rollup pattern (`internal/api/handler/status.go`) was considered and
  rejected for this piece specifically, because `GET /api/v1/status` is
  served by the very process whose health it is answering: it can report on
  itself live, with no storage in between.
- Keep the existing 60s page-level cache (`web/src/routes/status/+page.server.ts`)
  as-is; no separate uncached client-side fetch.

`internal/platform/observability` already runs two Prometheus counters
(`freehire_http_requests_total`, `freehire_http_route_requests_total`) fed
by `HTTPMetrics()` (normal completion) and `CountErrors()` (error-handler
path) — see that file's comments for why the split exists (Fiber renders an
error's status only after middleware unwinds).

## Goals / Non-Goals

**Goals:**
- Answer "is the site itself working right now" on the same page that
  already answers "is the ingest fleet working."
- Reuse signal the process already computes; add no new external calls.

**Non-Goals:**
- Historical/long-window uptime reporting (e.g. "99.9% over 30 days") — the
  in-process window resets on every deploy by design; that is a different
  feature with a different (persistent) data model, not attempted here.
- Alerting/paging — that is `deploy/bin/site-alert.sh`'s job already, and
  out of scope.

## Decisions

**In-process rolling window vs. querying Prometheus.** The process serving
`/api/v1/status` already increments the two Prometheus counters, but reading
them back requires either an HTTP round-trip to `/metrics` (same process,
same information, just serialized through Prometheus's text format and
re-parsed — pure overhead) or a real Prometheus query (rejected above,
cross-repo, adds latency and a failure mode having nothing to do with the
site's actual health). A package-level `requestWindow` fed by the same
call site as the Prometheus counters gives the identical information with
no serialization and no external call. Trade-off: state resets on deploy
and is per-process rather than fleet-wide — acceptable, because the process
answering the request IS the fleet member currently taking traffic (blue/
green: the cold color never receives a `/status` request to answer with its
own stale/empty window).

**Threshold-based classification, mirroring the existing ingest-status
pattern.** `deriveStatus`/`fleetStatus` in `status.go` already express "how
healthy is X" as a pure function over named constants with a minimum-signal
guard (fleet status won't red on one tiny provider). `deriveSiteStatus`
follows the same shape: a minimum-traffic guard before trusting the error
fraction at all (so one 5xx out of two requests right after a deploy doesn't
read as an outage), then two thresholds (`degraded`, `down`) over the error
fraction, with a live DB ping short-circuiting straight to `down`. Concrete
threshold values (minimum traffic, degraded/down error fractions, window
width) are implementation constants chosen conservatively and adjustable
without a spec change — the spec constrains the shape of the rule, not the
numbers.

**DB check reuses `Health`'s approach, not `Health` itself.** `internal/api/
handler/health.go`'s `Health` handler is mounted on `*API`, not
`*statsHandlers` (the struct `IngestStatus` lives on), and pulling in the
whole `*API` type would cross a dependency boundary this handler doesn't
otherwise need. `statsHandlers` gains its own `*pgxpool.Pool` field, wired
from the same `cfg.Pool` `Health`'s registration already uses, and calls
`pool.Ping` directly — same technique, no shared code to fork later if the
two ever diverge.

**Endpoint stays `IngestStatus`/`/api/v1/status`, gains a field.** Renaming
the Go method or splitting into a second endpoint would touch call sites
and the page's existing single round-trip for no behavioral gain; the
existing envelope already composes independent verdicts (`overall` is a
fleet aggregate already distinct from any single provider), so adding a
sibling `site` object is consistent with the existing shape rather than a
special case bolted on.

## Database check ordering (found during manual verification)

The first implementation ran the ingest-fleet queries (`ProviderHealthRollup`,
`LatestOpenJobAddedAt`) before the database ping and returned their error
directly on failure — i.e. a full database outage made `GET /api/v1/status`
answer `500` with no body at all, in exactly the scenario the new site-status
section exists to report. Manually stopping the database container during
verification reproduced this immediately. Fixed by pinging the database
FIRST and short-circuiting to a `200` response (`overall: "down"`, empty
`providers`, `site.status: "down"`) when it fails, skipping the ingest
queries entirely (they share the same pool and would only fail the same
way). Re-verified: the endpoint now degrades gracefully instead of 500ing,
and recovers correctly once the database answers again.

This isn't covered by an automated test: simulating "the database goes down
mid-request" against the concrete `*pgxpool.Pool` the handler holds would
need either a fake pool behind a new interface seam or a testcontainers
stop/start cycle, either more machinery than this one branch justifies. The
pure classifier (`deriveSiteStatus`, `dbUp == false` → `down`) is unit
tested; this note plus the manual verification is the coverage for the
wiring around it.

## Code review outcome

A review pass (see tasks.md §5.3) raised ten points. Three led to real fixes,
kept minimal:

- **`overall` was wrongly set to `"down"` in the database-outage
  short-circuit.** A database hiccup makes the ingest fleet's health
  UNKNOWN, not failed — the fix reports the same `"operational"` + empty
  `providers` the app already uses for a genuinely empty rollup (see
  `fleetStatus`'s own "empty fleet is operational" comment), so `site` and
  `overall` stay the independent signals the design intends. `site.status`
  still correctly reports `"down"`.
- **`requestWindow` only pruned old buckets inside `errorRate()`.** If
  `/api/v1/status` ever goes unpolled for a stretch while traffic keeps
  flowing, buckets would grow one-per-elapsed-minute with nothing to trim
  them. Fixed by pruning in `record()` too, against a generous
  `maxBucketAge` (1h) independent of any caller's query window.
- **`HTTPMetrics`/`CountErrors` each repeated the same three lines**
  (two counter increments + `RecordRequest`) after this change added the
  third. Extracted into one `recordResponse` helper both call, so a future
  addition to what gets recorded per response has one call site instead of
  two kept in sync by hand.

The rest were considered and deliberately left as they are, since each was
already a reasoned trade-off rather than an oversight:

- **The existing 60s page-level cache also covers the new live `site`
  signal**, so a visitor can see a stale up/down verdict for up to a
  minute. Already discussed and accepted before implementation (see
  Context above) — the user explicitly chose to keep the one existing
  cache rather than add a second, uncached fetch path for this section
  alone. The trade-off is real (a status page answering slightly stale)
  but matches how the same page already treats the ingest-fleet rollup,
  and how public status pages generally behave.
- **A database outage answers `200` with `site.status: "down"` in the
  body, rather than `503` like `/health`.** Deliberate: this is a status
  read whose entire point is to hand the FRONTEND a renderable verdict
  even when things are broken, the same way `IngestStatus` already reports
  `"down"` providers with a `200` rather than erroring. Nothing in this
  repository polls `/api/v1/status`'s HTTP status for alerting today
  (`site-alert.sh` polls `/health` and `/api/v1/jobs`) — if that ever
  changes, the status code and the body's `site.status` need to be
  reconciled then, not now.
- **The minimum-traffic guard (`minSiteRequestsForSignal`) can mask a bad
  deploy's first ~19 requests.** Already named as an accepted trade-off in
  Risks below, for the same reason repeated there: the opposite failure
  (flagging `degraded` off a couple of unlucky requests) is the more
  likely false positive at freehire's actual traffic volume.
- **`pool.Ping` duplicates `Health`'s liveness check rather than sharing
  it, and adds one DB round-trip per request.** Already a named Decision
  above (`statsHandlers` isn't `*API`, and a fork here has no shared code
  to diverge later); `pool.Ping` is cheap enough on a pooled connection
  that the extra round-trip is not a real cost next to the two queries the
  healthy path already runs against the same pool.
- **A single failing route can be diluted below the error-rate thresholds
  by a high-traffic healthy majority.** This mirrors the SAME aggregate
  choice `internal/platform/observability`'s own Prometheus counters
  already make deliberately (see that package's comments on why they
  don't label by route) — matching an existing, already-justified
  architectural stance rather than a new gap.
- **`statusResponse` takes six positional parameters.** Two call sites,
  distinct types per position; a DTO struct would be marginally more
  self-documenting but is not worth the indirection at this size.

## Risks / Trade-offs

- **Window resets on deploy** → a fresh deploy briefly shows `operational`
  with a near-zero total even if the previous process was struggling, and a
  deploy itself can't be flagged as "degraded" by this signal. Accepted: the
  DB-ping check still catches a hard failure immediately, and this feature
  is explicitly not the alerting path (`site-alert.sh` already covers that
  case on a 2-minute poll).
- **Per-process, not fleet-wide** → in blue/green, only the color currently
  serving traffic (and therefore answering `/status`) is measured; the cold
  color is invisible to this signal. Accepted per the goal: the page answers
  "is the site working for a visitor right now," which is exactly the
  active color's own view.
- **Minimum-traffic guard can mask a real problem on a very-low-traffic
  deployment** → acceptable for freehire's actual traffic volume; the guard
  exists specifically to avoid the opposite failure (flagging `degraded` off
  two unlucky requests), which is the more likely false positive in
  practice.
