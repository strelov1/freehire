# Fit-analysis use cases

## Scope

`internal/candidate/fitanalysis` orchestrates the fit analysis: the per-`(candidate, job)` cache,
the staleness stamp, the AI-credit rule, and the coalescing that stops two concurrent callers
paying for two chains. The chain itself, the `Analysis` type and its sanitize ceilings stay in
[`internal/candidate/matchanalysis`](../matchanalysis/AGENTS.md) — this package uses that domain,
it does not replace it.

## Why it exists

The chain has four entry points and only one of them speaks HTTP:

| Caller | Enters through | Charged? |
|---|---|---|
| `GET /jobs/:slug/match-analysis` | `Cached` | never (no LLM at all) |
| `POST /jobs/:slug/match-analysis` | `Authorize` → `Run` | first analysis of that job only |
| `GET …/match-analysis/stream` | `Authorize` → `Claim` → `Run` or `Follow` | same rule, per caller |
| the autopilot's cold-start fill and post-run refresh | `Ensure` / `Refresh` | **never** |

The last two execute **after** the request, from a detached SSE-writer goroutine, with no fiber
ctx at all. A rule written where one caller happens to reach it is a rule the other three never
meet — which is how the metering, the stamp and the coalescing came to live on a Fiber handler,
reachable only through a `*fiber.Ctx`.

## Always true

- **Only a FIRST analysis of a job is chargeable.** A recompute is always free, so an analysis
  cached before credits shipped re-runs for nothing. `Authorize` decides it; `Request.Chargeable`
  carries the answer.
- **Every caller gates for ITSELF, leader or follower.** Two tabs on one never-analysed job each
  genuinely owe a credit — following someone else's compute is not a discount. Charging is
  idempotent per `(candidate, feature, job)` (`credits.Store.Debit`'s `DebitExists`), so two
  callers that both decide "I owe one here" collapse into one ledger row.
- **The autopilot's two halves never charge.** `Ensure` ignores `Chargeable` rather than trusting
  it. Their spend is tracked only by the LLM attribution every call already carries.
- **The gate runs before the LLM and before a stream's headers.** `Authorize` refuses with
  `*InsufficientCreditsError`, carrying the balance — the caller renders the 402. An out-of-credits
  request must never become an event on a stream that already returned 200.
- **The cache stores the UNCAPPED analysis.** The hard-constraint ceiling is recomputed and applied
  to the served copy by the caller, so a dictionary change takes effect with no cache invalidation.
- **A leader MUST release its claim exactly once**, on every outcome including "LLM unconfigured",
  or every follower blocks forever. `Run` and `Ensure` do it from a defer, so a panic still wakes
  the followers. `Release` is idempotent — a double call must not be a double-close panic, which on
  the SSE writer goroutine (fasthttp recovers no panics there) would take the process down.
- **A follower checks the leader's success before trusting the cache.** A failed leader leaves an
  OLDER or absent row, and serving it would dress a stale analysis up as this run's live result.
- **Best-effort where it says so:** the cache write and the debit are logged, never surfaced — the
  analysis is already computed by then. `Balance` answers nil on any failure; the atomic `Debit`
  is the real ceiling, not the pre-check.

## Ports

- `Store` — the analysis cache. `*db.Queries` satisfies it. It trades generated row types
  deliberately, the same way `internal/ai/embed`'s ports do.
- `Meter` — the AI-points ledger. `*credits.Store` satisfies it. **A nil `Meter` is a working
  no-op** (balance unknown, nothing charged) so a fixture without a ledger still runs the chain.
  Pass a nil *interface*, never a non-nil interface holding a nil `*credits.Store` — see
  `meterOrNil` in the handler.
- `Request.Analyzer` — the per-candidate bound analyzer. Bound by the CALLER, because minting a
  gateway credential is a network call that a streaming caller must make before its headers go out.
  The service's own analyzer is the unbound one and answers only `ModelID`.

## Staleness

`Stamps` is quadruple: CV upload time, job `content_hash`, model, profile language. `Fresh`
compares live against stored. A `content_hash` absent on **both** sides counts as unchanged
(a non-board job is never re-crawled); present on one side only is a change. The model stamp
invalidates on an `LLM_MODEL` upgrade; the language stamp on a profile language switch
(freehire#1837).

The single-job read and the analysed-jobs listing both go through `Fresh`, so the list cannot
disagree with the page it links to. `Cached` returns the stored stamps rather than a ready-made
verdict: most jobs a candidate opens were never analysed, and reading a profile language to date
an analysis that does not exist would put a query on the commonest read there is.

## Gotchas

- `Refresh` claims nothing, on purpose: by the time it runs the turn is over, so there is no
  concurrent caller for that pair to coalesce with.
- `Run` with a nil `emit` runs the sync chain; non-nil streams it. That is the only difference
  between the on-demand endpoint and the streamed one.
- A nil analysis with a nil error means the LLM is unconfigured — nothing computed, nothing
  cached. Every caller degrades to "no analysis" rather than an error.
