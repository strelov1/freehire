# internal/engage/socialdigest

Builds the daily "most viewed postings" list and hands it to whatever publishes it.
Drained by [cmd/social-digest](../../../cmd/social-digest/main.go), once a day.

## The one rule that matters

**Rank on `page_uniques`, never on `uniques`.** `uniques` fuses two signals
[`viewlog`](../../application/viewlog/AGENTS.md) counts: page opens, which are
filtered against a known-bot list, and API reads, which deliberately are not — the
API exists to be read by programs. Crawlers are most of this host's traffic, so a
public "most popular" list built on `uniques` publishes what robots fetched as though
it were what people liked. Migration `0138` added `page_uniques` beside `uniques`
rather than redefining it, because `uniques` is what `GET /api/v1/stats/catalog`
already publishes.

## Shape

- `Select(candidates, quarantined) []Posting` — the editorial rules, a pure function
  over a slice. The cap applies **before** the truncation to `Size`, so a company's
  third posting yields its place and the list comes out full; truncating first would
  leave a hole nothing refills. Quarantine is checked before the cap is counted, so a
  quarantined posting does not spend its company's place.
- `ResolveDay(latest, hasData, now)` — the day is **discovered**, not computed from
  the clock. `cmd/rollup-views` fires at 02:30 UTC and reads the *rotated* access log,
  so whether its freshest complete day is yesterday or the day before depends on when
  logrotate runs on the host — an assumption that would fail by silently publishing a
  stale list. Past `StaleAfterDays` it returns `ErrStaleViewData` rather than a day.
- `Service.Build` / `Service.Dispatch` — assemble, then deliver. Every publisher is
  attempted even after one fails, and the failures are joined: a Discord outage must
  not cost the day everywhere, and the log should name every channel that broke.
- `Publisher` — `Name`/`Render`/`Publish`. `Render` returns exactly what `Publish`
  would send, because what a dry run is for is catching a list that **reads** badly,
  and a summary of a post cannot read badly.
- `Repository` — an interface, so the rules test without a database.

## Conventions

- **The editorial constants are constants, not configuration.** `MinPageUniques`,
  `QuarantineDays`, `MaxPerCompany`, `Size`. Each decides what the public sees under
  our own name, so changing one should be a reviewed commit rather than an env var
  edited over SSH.
- **The ledger is keyed `(day, channel)`.** A run that posts to one channel and fails
  on another must, next time, skip the first and retry the second. The quarantine
  reads that same ledger **across** channels: the list is the editorial unit, the
  channel is only how it is delivered.
- **The ledger is written after a successful publish, never before.** The two cannot
  be one transaction across an HTTP boundary, so one order has to lose. Recording
  first risks a day silently never published; recording after risks one duplicate
  post. A duplicate is visible and recoverable; a silent gap is neither.
- **An empty digest is a quiet day, not a failure** — publish nothing, exit zero.
  `ErrNoViewData` and `ErrStaleViewData` are the other thing: the pipeline is broken
  and must not be published over.
- **A channel with no credential is not configured**, so it is absent from the
  publisher list rather than present and disabled — the same degradation as the rest
  of this worker fleet.

## Not here

A LinkedIn publisher. Its Community Management API access request is filed and
awaiting review; until it clears there is no organization URN and no token, so the
code would ship and never run. The `Publisher` seam and the channel-keyed ledger
exist so that adding it is one file. Note that its access token lasts 60 days, so
that change owes a refresh worker as well — a Discord webhook URL never expires,
which is most of why Discord went first.
