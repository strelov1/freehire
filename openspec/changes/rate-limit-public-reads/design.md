## Context

`internal/ratelimit` is already the single rate-limit facility: a `Throttler`
interface, one Redis-backed GCRA implementation via `redis_rate`, a Fiber
`Middleware`, and two key builders (`KeyByIP`, `KeyByUserOrIP`). Eighteen routes
use it. None of them is a public read.

Everything this change needs is nearly there. The one structural obstacle is
that `Allow` throws away most of what the backend returns.

## Decision 1: widen `Throttler.Allow` rather than add a second method

```go
Allow(ctx, key string, limit int, window time.Duration) (bool, time.Duration, error)
```

`redis_rate.Result` already carries `Remaining` and `ResetAfter` alongside
`RetryAfter`; our signature discards both. Headers cannot be built from what the
interface returns, so the interface changes:

```go
type Decision struct {
    Allowed    bool
    Limit      int           // the ceiling in force, echoed for X-RateLimit-Limit
    Remaining  int           // requests left in the window
    ResetAfter time.Duration // until the bucket is full again
    RetryAfter time.Duration // meaningful only when !Allowed
}

Allow(ctx, key string, limit int, window time.Duration) (Decision, error)
```

A struct rather than a fourth and fifth return value: five positional results at
a call site is where argument-order bugs live, and the set will grow again if a
tier ever needs a name attached.

The alternative — keep `Allow` and add `AllowDetailed` — was rejected. Two
methods with one implementation means every fake implements both or embeds a
default, and the header path silently regresses the moment someone calls the
older one. There are five implementations in the tree (one real, four test
fakes) and all five are inside `internal/ratelimit`, so the widening does not
leave the package.

## Decision 2: two budgets, split by measured cost

| class | endpoints | measured peak/IP | ceiling |
|---|---|---|---|
| cheap | `/jobs/search`, `/jobs/facets`, `/jobs/{slug}`, `/jobs/{slug}/similar`, `/companies`, `/companies/{slug}`, `/geo/cities` | 258/min | 600/min |
| expensive | `/agent/jobs/search` | 184/min | 300/min |

Peaks are per-IP per-minute over a 4.6-hour production window (419,957 log
lines). The split is not aesthetic: `agent/jobs/search` rehydrates every hit's
full description from Postgres, which measures ~7x the ordinary search in bytes
and ~2.4x in latency. A single budget would have to be set for the expensive
endpoint and would then throttle facet lookups seven times harder than their
cost warrants.

600/min is 10 r/s — deliberately the same number nginx already enforces on the
HTML pages, so the site has one ceiling to explain rather than two.

Keys use `KeyByUserOrIP`, not `KeyByIP`. Not for tiering — the budget is
identical either way — but for correctness: a signed-in user behind a corporate
NAT should not share a bucket with strangers who happen to share their egress
address.

## Decision 3: exempt trusted peers, and do it in the middleware

SSR calls the API at `http://127.0.0.1:8081` (`API_INTERNAL_URL`), bypassing
nginx. `web/src/lib/server/api.ts` forwards the session cookie and nothing else,
so no `X-Real-IP` reaches Fiber. With `ProxyHeader: "X-Real-IP"` and a trusted
peer, Fiber's `extractIPFromHeader` finds no header and falls back to the socket
address: **every server-rendered page keys to `127.0.0.1`**. A per-IP budget
would put the entire website in one bucket and throttle it within seconds of a
traffic spike.

Two ways out; the second is rejected:

1. **Skip the limiter when the peer is loopback or private.** The limiter exists
   to bound external abuse. If our own SSR ever floods the API that is a bug in
   the SSR, and a 429 to our own page renderer is a worse symptom than the flood.
2. Have SSR forward `getClientAddress()` as `X-Real-IP`. Conceptually tidier —
   each visitor spends their own budget — but it makes a human clicking through
   the site draw from the same allowance as a scraper, which is exactly the
   false positive the ceiling was chosen to avoid. It also spreads the change
   across two languages for no gain the origin can feel.

The exemption reuses the CIDR list already in `cmd/server/main.go:123`
(`10/8`, `172.16/12`, `192.168/16`, `127.0.0.1/32`), lifted into the ratelimit
package so the two cannot drift: a peer Fiber trusts to set `X-Real-IP` is
exactly a peer we are not defending against.

## Decision 4: headers on every limited response, not only on rejection

`X-RateLimit-*` goes on the 200s too. A ceiling disclosed only at the moment of
rejection is not a contract, it is a surprise — the integrator report that
prompted this change specifically noted the *absence* of headers, not the
presence of throttling. A client that reads `Remaining` can slow down before it
is refused; one that only sees `Retry-After` has already failed a request.

`Retry-After` stays exactly as it is on 429, including the existing floor to one
second so a sub-second value never rounds to `0` and invites an immediate retry
into a second denial.

## Risks

**A ceiling too low breaks a real integrator silently.** Mitigated by deriving
both numbers from measured peaks with headroom (2.3x and 1.6x), and by logging
every 429 with its user agent and path so "who did this catch" is answerable a
week later without new instrumentation.

**Redis outage becomes a site outage.** It does not: `Middleware` already fails
open on any `Allow` error and on its own 100 ms timeout, and that path is
untouched. A fail-open response carries no `X-RateLimit-*` headers — absent is
honest, invented numbers are not.

**The exemption is a bypass if the trust boundary is wrong.** It is the same
boundary Fiber already uses to decide whether to believe `X-Real-IP`; if that
list were wrong, IP-based limiting would already be spoofable and the auth
limiters already broken. This change does not widen it.
