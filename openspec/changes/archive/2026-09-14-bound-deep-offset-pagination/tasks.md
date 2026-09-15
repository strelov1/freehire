## 1. The shared window

- [x] 1.1 `maxPageWindow` in `handler.go`; `pageParamsWindowed` refuses `offset+limit > maxPageWindow` with 400 "pagination too deep".
- [x] 1.2 `search.go` and `swipe.go` drop their own `maxSearchWindow` check and call the helper, so one constant decides for every store.
- [x] 1.3 Unit test: the helper allows the boundary (`offset+limit == maxPageWindow`), refuses one past it, and still clamps a 64-bit offset into int32 range rather than wrapping negative. Mutation-checked — disabling the guard fails three of the five.

## 2. The five unbounded Postgres lists

- [x] 2.1 `GET /api/v1/jobs` (`jobs.go`).
- [x] 2.2 `GET /api/v1/companies` (`companies.go`).
- [x] 2.3 `GET /api/v1/companies/:slug` — the embedded job list (`companies.go`). The window moved ABOVE the company lookup: a refused request must cost no query at all.
- [x] 2.4 `GET /api/v1/jobs/:slug/copies` (`copies.go`) — keeps its own limit ceiling, gains the window, same reordering.
- [x] 2.5 `GET /api/v1/companies/:slug/feedback` (`company_feedback.go`).
- [x] 2.6 `pagination_window_routes_test.go` drives all five through their real registers with zero-valued handlers, so a 400 PROVES nothing was queried. Carries its own control (a shallow request must reach the handler) so a path typo cannot make it pass on nothing. Mutation-checked — all five fail without the guard.

## 3. The missing limiter

- [x] 3.1 `GET /api/v1/companies/:slug/feedback` mounts `publicReadLimiter` — the one public list that had none.
- [x] 3.2 Split into `registerPublic`, following `mentorshipHandlers`: the existing guard drives every GET a register mounts and requires the limiter to LEAD each chain, which this feature's cookie-gated and moderator reads cannot satisfy. Added to `publicReadRoutes`, so its key is now checked against what the mounted chain can actually see.

## 4. The backstop: a query may not hold a connection forever

- [x] 4.1 `database.WithStatementTimeout`, applied by `cmd/server` only at 30s. Opt-in, because the cron workers share the package and `backfill-derive` legitimately runs for hours.
- [x] 4.2 Test: the option reaches `RuntimeParams` as milliseconds (a unitless "30" would be 30ms — set-looking and cutting every real query), a pool without it carries none, and applying it does not disturb the connection cap.

## 5. The status page sees a saturated pool

- [x] 5.1 `currentSiteHealth` reads `pool.Stat()` and reports the fraction as `pool_pressure`.

      **This shipped REVERSED from how it was written, and the reversal is the finding.** The
      task said "at or above 90% of the pool held, the site reads `degraded`", and that was
      built — then removed before it could do harm. `StartSiteStatusSampler` takes ONE reading
      every five minutes and `RecordSiteStatusSample` keeps the day's WORST severity, while the
      live pool — sampled every 5s against the healthy production site — reads 9/10 and 10/10
      inside the same two minutes it otherwise spends at 0/10. Real traffic is bursty and
      touching the ceiling is ordinary, so one unlucky sample would have painted a whole day
      degraded and the 90-day history strip would have gone yellow permanently, with no way to
      walk it back.

      An instant cannot carry that verdict. The number is reported and the Grafana rule judges
      it, averaging over five minutes (0.125-0.235 healthy against the outage's sustained 1.0) —
      history this process does not keep. Same lesson as §7.1's three drafts, found the same
      way: by measuring the live pool instead of reasoning about it.
- [x] 5.2 Tests pin the arithmetic and the division guard (a pool reporting no capacity yields 0,
      because "I cannot measure this" must not render as "everything is held"), plus the decision
      itself: an exhausted pool must read `operational` from `deriveSiteStatus`.
- [x] 5.3 `pool_pressure` on the wire; `StatusBoard.svelte` reports it beside the error rate,
      unconditionally. An earlier draft showed the line only above 90% and phrased it as a
      warning — which would have cried wolf on the same ordinary bursts.

## 6. Docs

- [x] 6.1 `docs/API.md` and `web/static/openapi.yaml`: the window applies to every list, the refusal is deliberate, and it is not a substitute for the rate limit.
- [x] 6.2 `internal/api/handler/AGENTS.md`: a caller-controlled offset is a caller-controlled COST, and a rate limiter cannot bound it.

## 7. Metrics and alerts

The outage was found by a person noticing the site was slow. Every signal that could have
named it existed somewhere and none of it was watched, so this section is what turns the
change from "this specific hole is closed" into "the next one is visible".

- [x] 7.1 `observability.NewPoolCollector` publishes acquired / idle / max, `EmptyAcquireCount`
      and `AcquireDuration`. Nothing read `pool.Stat()` before this. A Collector, not a polled
      gauge: it reads at scrape time, so there is no interval to choose.

      The two wait metrics are not interchangeable, and finding that out cost a second pass.
      `EmptyAcquireCount` was meant to be the alerting signal — "rises only when a caller found
      nothing free" — and on production it measured **15/s against a pool one-tenth occupied**,
      because pgx counts an acquire that waited at all, microseconds included. It describes
      concurrency, not a bottleneck. `AcquireDuration` is the one that carries a verdict: its
      per-second rate is dimensionless and exact — seconds waited per second elapsed IS the
      average number of callers queued. Ten thousand waits of a microsecond and ten waits of two
      minutes are indistinguishable by count and obvious by duration.
- [x] 7.2 `freehire_http_request_duration_seconds`, by route pattern. `requestBucket` carries
      `minute`/`total`/`errors` and had no field a duration could go into, so p95 was
      unanswerable — which is why "slow" was invisible until it became "down". Buckets stop at
      30s because that is now the pool's `statement_timeout`.
- [x] 7.3 Three rules in `freehire-ops` (`freehire-api-saturation`): pool starvation
      (critical), p95 latency (warning), and deep-pagination refusals (warning, 15m — the
      guard working is worth knowing about and must never wake anybody).
- [x] 7.4 `scripts/check-alert-rules.py` passes: 23 rules, UIDs within 40 chars, no `<` in an
      annotation. The pool rule's runbook query uses `!=` for exactly that reason.

## 8. Verification

- [x] 8.1 `gofmt -l .` clean, `go vet ./...` clean, `go vet -tags=integration ./...` clean,
      `go test ./...` — 226 packages ok, 0 failures.
- [x] 8.2 `perf/k6/deepoffset.js` — one VU, strictly sequential, its own `FORCE_DEEP_OFFSET`
      latch. Run against BOTH colours on the prod host while blue still held the pre-fix
      commit and green the fixed one: same host, same Postgres, the two versions side by side.

      | offset | before (`564024c3`) | after (`07b63825d`) |
      |---|---|---|
      | 0 | 2.193s | 3.334s |
      | 1,000 | 4.336s | 0.871s |
      | 10,000 | 3.987s | **0.211s refused** |
      | 50,000 | 20.243s | **0.211s refused** |
      | 179,500 | **53.968s** | **0.239s refused** |

      The climbing curve is the defect; it is gone. 54s → 0.24s, and no database work at all.
- [x] 8.3 Deployed (autodeploy, `07b63825d`). All five Postgres lists plus the Meili search
      answer 400 in 0.26–0.80s at `offset=179500`; the boundary page (`offset=9900&limit=100`)
      still serves 200, and the ordinary first page answers in 0.96s. The new pool and latency
      metrics are live on the active colour's `/metrics`.
- [x] 8.4 Alert rules live on litellm-host, shipped after the binary as the deploy-order note
      requires. Grafana logged `starting to provision alerting` → `finished to provision
      alerting` with nothing between; `alert_rule` holds 23 rules including the three new ones;
      no evaluation errors; `alert_instance` is empty, so nothing is firing falsely.

      Every expression was checked against live Prometheus BEFORE shipping, which is what a
      `noDataState: Alerting` rule demands: pool occupancy 0.235 (threshold 0.7), p95 latency
      0.53s (threshold 2s), 400-rate 0.034/s (threshold 2/s).

      Getting the pool expression right took three drafts and two live measurements, and both
      discarded ones looked correct on paper. `rate(empty_acquire_total)` read 15–39/s on a pool
      one tenth occupied — pgx counts an acquire that waited at all, microseconds included.
      `rate(acquire_seconds_total)` read 1.36 s/s on that same idle pool, and the claim that
      this was "the average number of callers queued" was wrong: pgx's `AcquireDuration` is the
      total duration of ALL acquires, instant hand-offs included. What works is averaged
      occupancy — instantaneous `acquired/max` is bursty (sampled 9/10, 10/10, then 0/10 for
      most of two minutes), but over five minutes it settles at 0.125–0.235 while the outage
      held 10/10 for fifty minutes.
