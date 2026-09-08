## Context

See proposal.md for the measured root cause. Summary of the transport facts, all
measured live on prod on 2026-09-08 (`ssh root@89.167.94.146`, via
`/opt/freehire/.env`'s `SOURCES_PROXY_URL`/`FIRECRAWL_API_KEY`):

| path | transport | result |
|---|---|---|
| listing (`hh.ru/search/vacancy`) | direct datacenter IP | 200, `HH-Lux-InitialState` present |
| listing | proxy | 200 (this is what's live today; jobs/titles/companies land fine) |
| detail (`hh.ru/vacancy/<id>`) | direct datacenter IP | 200, 80/80 sustained at ~4 req/s, full `JobPosting` ld+json |
| detail | proxy | 302 → `/account/captcha` (DDoS-Guard image CAPTCHA), 40/40 live prod failures |
| detail | Firecrawl | 200 target status, 1/1, full description (~2900 chars) |

`hh` (`internal/ingest/sources/hhru.go`) currently uses ONE `http HTMLGetter` field
for both `crawl()` (listing) and `detail()`, and `proxy.go`'s `proxiedProviders["hh"]`
routes that single field through `pacedHTMLGetter(proxiedClient, hhRequestInterval,
hhRequestBurst)` — both paths share one transport and one rate limiter.

`internal/ingest/sources/firecrawltier.go` already has the hosted-fetch plumbing:
`firecrawlClient` implements `HTMLGetter`/`XMLGetter` over `internal/platform/firecrawl`,
so an adapter that already takes an `HTMLGetter` needs no new client code — only a
way to hand it a DIFFERENT getter than the one used for listing. `wantapply` is the
existing precedent for a split transport (`NewWantapplyViaHostedSitemap(direct,
hosted, ...)`), proving the `firecrawlProviders` build-function shape
(`func(hosted *firecrawlClient, direct HTTPClient) Source`) already supports handing
an adapter two transports.

hh's boards were onboarded 2026-09-03 (`boards` table, 5 `professional_role` rows,
all `active`). With `hhWithinDays = 7`, the crawl window only fully "settles" to
steady-state (no more 7-day backlog being walked as "new") around 2026-09-10.
Today's measured detail-fetch rate (~690/hour, 2938 in the log's ~4.26h retention
window) is very likely inflated by that backlog and will drop once it clears — by
how much is not yet known.

## Goals / Non-Goals

**Goals:**
- Stop hh postings from landing with a salary-only description by moving detail
  hydration off the currently-broken proxy path.
- Keep the change to exactly what's broken: detail hydration. Listing already
  works and is not touched beyond removing it from the (now pointless, for hh)
  proxy wiring.
- Leave the adapter's observable contract (Job shape, `ExternalID`, seen/hydrate/
  list-only-fallback behavior) unchanged — this is a transport swap, not a
  behavior change.
- Document the real, current transport facts in place of the stale "hh.ru's
  detail pages 403 the direct datacenter IP" comment, so the next reader trusts
  what's written rather than rediscovering this at their own expense (the
  existing convention in `firecrawltier.go`'s own comments).

**Non-Goals:**
- Not building CAPTCHA-solving of any kind (not in scope, and per the spike, an
  image CAPTCHA is not something a headless browser or a generic hosted-fetch
  vendor bypass solves anyway — see proposal.md's "Why").
- Not building a generic proxy-health-monitoring or automatic-tier-fallback
  mechanism. If hh's direct IP gets reblocked in the future, that's a new
  investigation, not something this change tries to predict or automate around.
- Not changing `bayt`/`gulftalent`/`wantapply`'s own firecrawl usage or budgets
  beyond raising the one shared `FIRECRAWL_MAX_PAGES_PER_RUN` ceiling they also
  read.
- Not committing to a specific Firecrawl paid plan tier in code — that's an
  account-level, deploy-side decision (see Migration Plan).

## Decisions

**1. Split `hh`'s single `HTMLGetter` into a listing getter and a detail getter.**

`hh` gets a second constructor, `NewHHWithDetailGetter(listing, detail HTMLGetter) Source`,
alongside the existing `NewHH(c HTMLGetter) Source` (kept as `NewHHWithDetailGetter(c, c)` —
same transport for both — so every other call site, and the default/no-firecrawl-key
registry entry in `registry.go`, is unchanged). `firecrawltier.go` registers hh's
build function using this new constructor.

*Alternative considered:* thread a single `HTMLGetter` but branch inside `detail()`
on some flag. Rejected — the existing wantapply precedent already establishes
"two transports, one adapter" as a constructor-level split, and threading a
runtime flag through one field would obscure which transport serves which path
instead of making it a type-level fact.

**2. Detail hydration moves to the Firecrawl tier; listing moves OFF the proxy
entirely and onto the plain direct client.**

`hh` is removed from `proxiedProviders` (`proxy.go`) and added to
`firecrawlProviders` (`firecrawltier.go`) as:
```go
"hh": func(hosted *firecrawlClient, _ HTTPClient) Source {
    return NewHHWithDetailGetter(NewClient(), hosted)
},
```
The build function deliberately ignores the `direct` parameter `ApplyFirecrawlEgress`
passes in — that value is ONE shared client for every mixed-tier provider in the
loop, and it becomes the PROXIED client whenever `SOURCES_PROXY_URL` is set at
all (which it is on prod, for eightfold/djinni/2gis/etc.), regardless of whether
the provider being built is itself in `proxiedProviders`. Threading it through
for hh would silently put hh's listing back on the very proxy IP this change
exists to stop using. hh's listing gets its own fresh `NewClient()` instead —
plain, unproxied, matching what was actually measured working (200, full
`HH-Lux-InitialState`) — independent of whatever proxy configuration exists for
other providers. (This is the one place `wantapply`'s existing shape — reuse the
shared `direct` — doesn't transfer: wantapply's non-hosted pages genuinely need
the proxy, hh's listing pages measurably don't.)

*Alternative considered:* keep listing on the proxy (leave `hh` in BOTH
`proxiedProviders` and `firecrawlProviders`). Rejected on two grounds: (a) no
evidence listing needs the proxy — it works identically on the direct IP right
now — so routing it through a proxy IP that's already burned for the SAME domain
buys nothing; (b) `ApplyFirecrawlEgress` runs LAST and unconditionally overwrites
`registry["hh"]`, so `ApplyProxyEgress`'s paced wiring would be silently discarded
anyway — keeping hh in both maps would be dead configuration, not a real
fallback.

**3. Drop hh's dedicated pacer (`hhRequestInterval`/`hhRequestBurst`,
`pacedHTMLGetter`).**

Neither remaining path needs it: `firecrawl.Client.Fetch` already has its own
vendor-side 429 handling (`errVendorRateLimited` + `retryWait`, shared with
bayt/gulftalent/wantapply), and listing's own loop (`crawl()`) issues at most
`hhMaxPages` (20) sequential requests per board — already naturally paced by
being sequential, and already measured clean at that shape on the direct IP.

*Alternative considered:* keep the pacer wrapped around the listing-only direct
client, in case sequential-but-fast listing requests draw attention on their
own. Rejected for now as speculative hardening with no measured need — per
AGENTS.md's "no MVP shortcuts, no overengineering," this can be added later
with a number attached if board_health or logs ever show listing being refused
on the direct IP, the same way every other pacer constant in `pacer.go` was
added after a measured failure, not ahead of one.

**4. `FIRECRAWL_MAX_PAGES_PER_RUN` needs raising on prod — deploy-side, not code.**

Each `cmd/ingest <provider>` invocation is its own process with its own
`ApplyFirecrawlEgress` call, so the budget is effectively per-provider-per-run
(one hh run never competes with a same-instant bayt run for the same budget —
they're different processes). hh's own hourly run needs headroom for its
current (backlog-inflated) rate of ~690 detail fetches/hour; recommend setting
`FIRECRAWL_MAX_PAGES_PER_RUN=2000` for real headroom against both the current
rate and any spike, high enough that a normal hh run never trips
`ErrBudgetSpent`. Because the same env var also caps bayt/gulftalent/wantapply's
own separate runs, raising it is a ceiling raise for them too, not a spend
increase — their actual usage is unaffected unless their own crawl volume grows
to need it.

Real cost, from proposal.md's measurement (Firecrawl's published pricing,
checked 2026-09-08): 1 credit/page (`formats:["rawHtml"]`). At today's
backlog-inflated ~497K credits/month, that's at the edge of the 500K/mo
"Growth" plan ($27.75/mo on annual billing); the 1M/mo "Scale" plan ($49.92/mo)
gives real headroom until the steady-state rate is known. This is an operator
decision on the Firecrawl account, not something this change can set in code —
called out as a deploy task.

## Risks / Trade-offs

- **hh's steady-state volume (and therefore steady-state cost) is not yet known**
  — today's measurement is inflated by the 7-day backlog since boards onboarded
  2026-09-03, which only fully clears around 2026-09-10.
  → Mitigation: size `FIRECRAWL_MAX_PAGES_PER_RUN` generously now (headroom, not
  a tight fit), pick the Firecrawl plan tier from today's conservative number,
  and re-check `internal/platform/firecrawl`'s spend once the backlog settles
  before assuming the number is stable long-term.

- **This introduces a real recurring paid cost where there was none before**
  (the current proxy is "free" in that it's a flat-rate egress already paid
  for regardless of hh).
  → Mitigation: already surfaced to and decided by the user with real pricing
  figures before this proposal was written (see proposal.md); `firecrawl.Client`
  already refuses a non-positive budget outright and claims budget BEFORE each
  request, so a misconfiguration fails loudly rather than silently overspending.

- **Listing was only spot-tested on the direct IP, not sustained-volume tested**
  (the 80/80 sustained test was for DETAIL pages; listing is at most 20
  sequential requests per board per run, much lower volume, but genuinely
  untested at that shape over many consecutive hourly runs).
  → Mitigation: task list includes watching `board_health` / hh's ingest logs
  for a few runs post-deploy; if listing ever gets refused on the direct IP,
  the fix is the same three-tier playbook this change already follows (route
  listing through proxy or firecrawl too) — this change doesn't foreclose that,
  it only stops using a transport that's currently broken for no benefit.

- **hh.ru's WAF posture could flip again** (it already did once, in the
  direction the original proxy comment describes) — direct IP could get
  reblocked for either listing or detail in the future.
  → Mitigation: this isn't preventable in advance; the existing three-tier
  pattern (`proxiedProviders`/`browserProviders`/`firecrawlProviders`) is
  exactly the playbook for re-diagnosing and re-routing if it happens, the same
  way this investigation used it.

## Migration Plan

1. Code change (this repo): `hhru.go`, `firecrawltier.go`, `proxy.go`, `pacer.go`
   (remove now-dead `hhRequestInterval`/`hhRequestBurst`), doc updates. Normal
   `gofmt`/`go vet`/`go test` gate, normal PR + `freehire-autodeploy` release —
   no schema or migration involved.
2. **Deploy-side, manual, AFTER the code deploys**: on host2, raise
   `FIRECRAWL_MAX_PAGES_PER_RUN` in `/opt/freehire/.env` (recommend 2000; see
   Decision 4) and confirm/upgrade the Firecrawl account's plan tier for the
   added volume. Both are called out as explicit tasks so they aren't missed —
   without them, hh's detail hydration will start hitting `ErrBudgetSpent` at
   the current default of 25/run almost immediately, which looks identical in
   the logs to today's captcha failures (list-only fallback), so this step is
   easy to silently skip and get the same symptom back.
3. Watch the next few hourly hh runs' logs (`journalctl -u
   freehire-ingest@hh.service`) for `hh: detail ... failed` lines dropping to
   near zero, and spot-check `/api/v1/jobs/search?source=hh` for real
   descriptions on newly-created rows.

**Rollback:** revert the code change. This returns hh to today's known (broken
but understood) state — proxy-routed detail hydration, salary-only fallback —
with no data migration involved either direction. Rows already hydrated via
Firecrawl keep their real descriptions; a rollback doesn't erase already-fixed
rows, it only stops fixing new ones.
