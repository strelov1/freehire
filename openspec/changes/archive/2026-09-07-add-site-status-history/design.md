## Context

See `proposal.md` - Why. This builds directly on the already-shipped
`add-site-status-to-status-page` change (archived at
`openspec/changes/archive/2026-09-07-add-site-status-to-status-page/`):
`internal/platform/observability`'s in-process `requestWindow`/`ErrorRate`,
`internal/api/handler/status.go`'s `deriveSiteStatus`/`siteHealth`/
`IngestStatus`, and `StatusBoard.svelte`'s "Site status" block. All decisions
below were confirmed with the user before this document was written; nothing
here is open for re-litigation without going back to them.

## Goals / Non-Goals

**Goals:**
- Show a 90-day daily history strip for the site's own status, matching the
  visual convention of real status pages.
- Never claim to know about a day nothing was actually observed on.

**Non-Goals:**
- Ingest-fleet history — `overall`/`providers` are unaffected.
- An uptime-percentage figure — only day-tile coloring by worst status.
- A new deployable (cron binary, systemd unit, `freehire-ops` change).
- Retention/pruning of `site_status_daily` — row growth is ~365/year;
  nothing deletes rows, ever.

## Decisions

**A ticker inside `cmd/server`, not a new cron worker.** The original
site-status feature deliberately avoided a cross-repo dependency
(`freehire-ops`) for a live signal the serving process could compute itself;
sampling that same signal every 5 minutes for history is the same call for
the same reason, and there is already a precedent for exactly this shape in
this codebase: `startSuggestRefresh` in `cmd/server/main.go` — a bare
`go func()` with `time.NewTicker`, driven by the server's own
`signal.NotifyContext` shutdown context, doing its first tick's work
immediately rather than waiting out the first interval. The new sampler
copies that shape. The alternative (a new `cmd/rollup-site-uptime` +
`freehire-ops` systemd timer) was rejected: it would leave the feature
incomplete from within this repo alone, since the timer unit lives in a
separate private repo this session cannot touch.

**Severity as an integer, not a status string, in storage.** The frontend
already orders `HealthStatus` by severity (`SEVERITY` map in
`StatusBoard.svelte`); the Go side gets a `providerStatus`-to-severity
mapping to match. Storing `worst_severity smallint` lets the "worse wins"
rule become one atomic SQL upsert —
`GREATEST(site_status_daily.worst_severity, EXCLUDED.worst_severity)` —
instead of a read-modify-write race between the two blue/green processes,
which independently tick and would otherwise need to coordinate. The read
path maps severity back to the wire's `operational`/`degraded`/`down`
strings, so nothing outside `status.go` ever sees the integer encoding.

**Absent days are absent, not zero-filled.** `site_status_daily` only gets a
row when a sample actually ran. A day with no row is omitted from
`site.history` entirely; the frontend renders the gap as its own "no data"
tile rather than defaulting to "operational." This matters concretely for
the 89 days immediately after this ships — the table starts empty, so
without this rule the page would show 89 fabricated green days on day one,
which is exactly the kind of dishonest "everything's always been fine" a
status page must not claim.

**The sampler reuses the handler's own computation, not a copy of it.** The
DB-ping + `observability.ErrorRate` + `deriveSiteStatus` assembly currently
lives inline inside `IngestStatus`. It moves into a small shared method
(e.g. `(h *statsHandlers) currentSiteHealth(ctx) siteHealth`), called by both
the HTTP handler and the new ticker, so the two can never quietly disagree
about what "the site's current status" means.

**Today's tile is an ordinary tile.** No "in progress" treatment — decided
with the user as unnecessary polish for this pass; today's tile simply shows
whatever the worst sample recorded so far today is, exactly like any other
day once it has at least one sample.

## Code review outcome

A review pass (see tasks.md §6.4) raised seven points. Five led to real
fixes:

- **`CURRENT_DATE` follows the Postgres session's timezone, not UTC**, while
  everything else in this feature (Go's `time.Now().UTC()`, the frontend's
  UTC-anchored `historyTiles`) treats a day as a UTC calendar date. This
  codebase already hit and fixed the identical class of bug once
  (`social_digest.sql`'s `LatestJobViewDay`, whose own comment names the
  exact failure: "a bare CURRENT_DATE would follow the session's timezone
  instead and, east of UTC, cut off a day that had not closed"). Fixed by
  switching both `site_status_daily.sql` queries to
  `(now() AT TIME ZONE 'utc')::date`, mirroring that precedent exactly.
- **The 90-day history window was actually 91 days.**
  `day >= CURRENT_DATE - INTERVAL '90 days'` is inclusive at both ends
  (today through today-90 = 91 distinct days), while the frontend's tile
  loop renders exactly 90 (anchor through anchor-89) — the oldest fetched
  day was silently dropped from view. Fixed by using `>` against the same
  90-day interval instead of `>=` against 89, which now reads the same as
  every "trailing 90 days" comment already says.
- **`StartSiteStatusSampler` built a throwaway, partially-populated
  `&statsHandlers{pool: pool}`** just to reach `currentSiteHealth` — a
  future change giving that method a reason to read `h.queries` or
  `h.cache` would nil-panic specifically on the sampler's path, invisibly
  to the HTTP handler's own tests. Fixed by turning `currentSiteHealth`
  into a plain function taking the `*pgxpool.Pool` it actually needs,
  called by both `IngestStatus` (`h.pool`) and the sampler (`pool`
  directly) — no incomplete struct anywhere.
- **Two independent switch statements for the same severity mapping**
  (`severityFromStatus`/`severityToStatus`), each a hand-kept copy that
  could drift if a fourth `providerStatus` value were ever added. Fixed by
  making `severityOrder` (a plain three-element slice) the one place the
  mapping is defined; both functions now derive from it.
- **`siteHistoryFromRows` hardcoded `"2006-01-02"`** instead of reusing the
  `dateLayout` constant already declared in `stats.go` and used identically
  by `insights.go`/`stats.go` elsewhere in the same package. Fixed to use
  `dateLayout`. Also folded the DB-down short-circuit's hand-built
  `[]siteHistoryEntry{}` into `siteHistoryFromRows(nil)`, so the "always
  `[]`, never `null`" invariant has one implementation instead of two.

Two points were considered and deliberately left as-is:

- **`IngestStatus` now runs three DB round trips sequentially**
  (`SiteStatusHistory`, `ProviderHealthRollup`, `LatestOpenJobAddedAt`)
  rather than concurrently via something like `errgroup`. The two
  pre-existing queries were already sequential before this change; adding
  concurrency now would restructure code this change didn't otherwise touch,
  for a latency micro-optimization on a handler with no reported latency
  problem. Left for a future change if it's ever measured to matter.
- Exhaustiveness tooling for `providerStatus`'s three-value switches
  (`deriveStatus`'s own switch, `severityOrder`, etc.) — this codebase does
  not use an exhaustiveness linter for string-typed "enums" anywhere today;
  introducing one here would be inconsistent with every existing switch over
  `providerStatus`, not a gap specific to this change.

## Risks / Trade-offs

- **A 5-minute sample interval can miss very short blips entirely**, or
  register a blip that resolved before the next tick as having happened
  without a lasting trace beyond that one day's tile. Accepted: this is the
  same resolution real status pages operate at, and catching sub-5-minute
  blips is what the existing LIVE pill (not the history strip) is for.
- **Both blue and green tick independently against the same table.** Not a
  bug: the upsert is commutative and idempotent regardless of which process's
  write lands first or last for a given tick round, and a cold standby's own
  clean reading (no real traffic, DB reachable) can only ever report
  `operational` for its own sample — it can never downgrade a real problem
  the active color already recorded, only (harmlessly) redundantly confirm
  health.
- **No sqlc regeneration tool may be available in a given environment.** If
  `make sqlc` cannot run here, the task list calls this out explicitly rather
  than hand-writing the generated Go, which would drift from the actual
  codegen output the moment the tool becomes available again.
