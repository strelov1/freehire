# Fit-analysis use cases

## Scope

`internal/candidate/fitanalysis` orchestrates the fit analysis: the per-`(candidate, job)` cache,
the staleness stamp, the plan-allowance rule, and the coalescing that stops two concurrent callers
paying for two chains. The chain itself, the `Analysis` type and its sanitize ceilings stay in
[`internal/candidate/matchanalysis`](../matchanalysis/AGENTS.md) — this package uses that domain,
it does not replace it.

## Why it exists

The chain has four entry points and only one of them speaks HTTP:

| Caller | Enters through | Charged? |
|---|---|---|
| `GET /jobs/:slug/match-analysis` | `Cached` | never (no LLM at all) |
| `POST /jobs/:slug/match-analysis` | `Reserve` → `Run` | first analysis of that job only, released if it produces nothing |
| `GET …/match-analysis/stream` | `Reserve` → `Claim` → `Run` or `Follow` | same rule, per caller |
| the autopilot's cold-start fill and post-run refresh | `Ensure` / `Refresh` | **never** |
| `GET /me/cvs/:id/tailor-context` + the agent's `cv_context` tool | `TailoringContext` | never (read only) |
| `GET /me/cvs/:id/job-match` + the agent's `job_match` tool | `Optional` | never (read only) |
| the agent's `interview_context` tool, the autopilot's run plan | `Required` | never (read only) |

The last two execute **after** the request, from a detached SSE-writer goroutine, with no fiber
ctx at all. A rule written where one caller happens to reach it is a rule the other three never
meet — which is how the metering, the stamp and the coalescing came to live on a Fiber handler,
reachable only through a `*fiber.Ctx`.

## Always true

- **The consumption IS the gate.** `Reserve` takes the allowance BEFORE the chain starts, so
  the counter's own row lock decides. It used to check what was left and charge afterwards,
  which is a check-then-act: two concurrent runs for two never-analysed jobs both passed a
  check only one of them could afford, and the loser's charge then failed silently — after its
  analysis had been computed, cached and served.
- **A run that produces nothing gives the allowance back.** `Run` and `Follow` release on every
  failure path, from a defer, so a panic returns it too. Taking it up front is only honest if a
  failed analysis still costs nothing.
- **A released ref is chargeable again**, which is what makes a retry cost one rather than none.
  `plan.Store.Release` RESTAMPS the row `kind='release'` instead of deleting it or appending a
  compensating one: `usage_ledger_consume_ref_uniq` is scoped to `kind='consume'`, so restamping
  frees the reference while keeping the fact that a reservation was taken.
- **Only a FIRST analysis of a job is chargeable.** A recompute is always free, so an analysis
  cached before metering shipped re-runs for nothing. `Request.Reserved` carries the answer.
- **Every caller reserves for ITSELF, leader or follower.** Two tabs on one never-analysed job
  are neither a double charge nor a discount: consumption is idempotent per
  `(candidate, feature, job)`, so both collapse into one ledger row.
- **Metering fails OPEN.** An unreachable counter logs and lets the run through unreserved.
  Bookkeeping must never be able to refuse a legitimate analysis, and an uncharged run is a
  smaller wrong than a candidate blocked by our accounting.
- **The autopilot's two halves never charge.** `Ensure` ignores `Reserved` rather than trusting
  it, so it reserves nothing and has nothing to release. Their spend is tracked only by the LLM attribution every call already carries.
- **The gate runs before the LLM and before a stream's headers.** `Reserve` refuses with
  `*RefusedError`, carrying the decision — the caller renders the 402. A refused request must
  never become an event on a stream that already returned 200.
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

## Reading a cached analysis

Three readers, three shapes, one rule about what "no analysis" means:

- **`Required`** — `ErrNoAnalysis` when there is none. For readers that cannot proceed.
- **`Optional`** — `(nil, false)`. For readers that carry on and say so in the result. A failed
  read degrades the same way but is **logged**: a broken database must not look like a candidate
  who never ran their analysis.
- **`Cached`** — the analysis plus the stamps it was written under, for the surface that reports
  staleness.

An unreadable stored blob reads exactly like an absent one. Both mean "there is nothing to ground
on, offer them a run", and a decode failure must not become a 500 on a surface whose honest answer
is "run the fit analysis first".

**`ErrNoAnalysis` never becomes a status here.** `internal/api/handler`'s `classify` maps it to 409
once, at the port boundary, so no call site invents its own code.

**A nil `*Service`, or one with no store, degrades rather than panicking** — `Required` answers
`ErrNoAnalysis`, `Optional` answers `false`, `Balance` answers `nil`. Agent tools run inside an SSE
writer's goroutine where a panic reaches no recover and takes the process down, so a partially
wired harness has to fail soft. The tools guard their other collaborators for the same reason.

## Ports

- `Store` — the analysis cache. `*db.Queries` satisfies it. It trades generated row types
  deliberately, the same way `internal/ai/embed`'s ports do.
- `Meter` — the plan allowance. `*plan.Store` satisfies it. **A nil `Meter` is a working
  no-op** (standing unknown, nothing charged) so a fixture without one still runs the chain.
  Pass a nil *interface*, never a non-nil interface holding a nil `*plan.Store` — see
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

- `ProjectTailoring` strips the posting's markup and clips it. The posting is the largest thing
  in a turn and the least trusted; sending markup spends the context window on tags and widens
  what there is to misread.
- `TailoringContext` takes the job rather than reading it, so this package needs no job port —
  every caller has already loaded and authorized that row. **It also takes the posting's own
  requirements** for a sharper reason: the model's enrichment list and the markup parser's
  `requirements_derived` column are reconciled in exactly one place, `jobview.FromDomain`, and
  `jobview` sits in the `job` block, which this one is BELOW and may not import. Merging them a
  second time here is precisely the drift that one-place rule exists to prevent, so the caller
  (`internal/api/handler`'s `postingRequirements`) does it and passes the answer in.
- **Two requirement lists reach a tailoring reader and they are not the same thing.**
  `MissingHave`/`MissingGap` is what an ANALYSIS decided the CV lacks; `Job.Requirements` is what
  the POSTING asks for, including what the CV already covers. The second matters because
  `descriptionLimit` clips the posting and a requirement list is nearly always at its end — so on
  a long posting the requirements were the first thing to fall off, and a reader that had no
  cached analysis saw none at all.
- `Refresh` claims nothing, on purpose: by the time it runs the turn is over, so there is no
  concurrent caller for that pair to coalesce with.
- `Run` with a nil `emit` runs the sync chain; non-nil streams it. That is the only difference
  between the on-demand endpoint and the streamed one.
- A nil analysis with a nil error means the LLM is unconfigured — nothing computed, nothing
  cached. Every caller degrades to "no analysis" rather than an error.
