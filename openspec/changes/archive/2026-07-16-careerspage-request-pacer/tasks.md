## 1. Rate-limited HTMLGetter decorator

- [x] 1.1 Add the `golang.org/x/time/rate` dependency (`go get golang.org/x/time/rate`; tidy).
- [x] 1.2 Implement a `rateLimitedHTMLGetter` in `internal/sources` wrapping an `HTMLGetter`
      plus a minimal `waiter` interface (`Wait(context.Context) error`, satisfied by
      `*rate.Limiter`); `GetHTML` calls `waiter.Wait(ctx)` (returning its error) before
      delegating. RED test first: an injected fake waiter records that `Wait` fires exactly
      once before each `GetHTML`, that a fake underlying getter is delegated to, and that a
      `Wait` error short-circuits (no delegation).

## 2. Wire careerspage through the pacer

- [x] 2.1 Add a named rate constant (e.g. `careerspageRequestInterval`/`careerspageRequestBurst`)
      and a small constructor that builds the shared limiter + wraps a getter, so one limiter
      is shared across a run.
- [x] 2.2 Wrap careerspage's client with the pacer at registry wiring so BOTH the direct
      (`All`) and proxied (`proxiedProviders`) careerspage paths get a paced getter, sharing
      one limiter per registry build; leave `careerspageDetailWorkers = 2` unchanged. Verify
      via `go build ./... && go test ./internal/sources/` that the registry still resolves
      careerspage and the suite is green.

## 3. Ops headroom

- [x] 3.1 Confirm the `freehire-ingest@careerspage` unit's `TimeoutStartSec` (in freehire-ops)
      has headroom for a paced run (bounded rate × ~100+ requests ≈ a couple of minutes); note
      the required value if a bump is needed. Docs/ops only — no app code. Confirmed:
      `TimeoutStartSec=2400` (40 min) — a paced run (~1.25 req/s × ~100+ requests ≈ 2 min) is
      well within it; no bump needed.
