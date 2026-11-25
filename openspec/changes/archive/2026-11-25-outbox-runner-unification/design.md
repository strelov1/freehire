## Context

Six Postgres-backed work queues drive six independent worker packages, each with
its own hand-rolled claim/lease/retry drain loop: `enrichment_outbox`
(`internal/enrich`), `semantic_outbox` (`internal/embed`), `search_outbox`
(`internal/searchdrain`), `apply_form_outbox` (`internal/applyform`),
`adzuna_description_outbox` (`internal/adzunadesc`), and
`email_classification_outbox` (`internal/maillink`). The six queue tables are
already structurally near-identical (`id, <subject>_id, attempts, claimed_at,
failed_at, last_error, created_at` + a partial claimable index), and the claim
loop was deliberately copied from package to package (SQL comments say "Mirrors
ClaimSemanticBatch", "Mirrors ClaimApplyFormBatch"). Full research (schema dumps,
runner code excerpts, inventory of all six queues) lives in
`docs/superpowers/specs/2026-08-09-outbox-runner-unification-design.md`; this
document carries the decisions forward into OpenSpec's tracked format.

## Goals / Non-Goals

**Goals:**
- Extract the duplicated claim-wave drain loop into one generic `internal/outbox`
  package, so a seventh queue only needs its own `Store` port and processing
  logic, not a seventh copy of the loop.
- Preserve every worker's exact external behavior: retry/dead-letter counts,
  lease timing, `worker.ExitCode` semantics, CLI/env surface.
- Migrate all six existing workers onto it in this one change.

**Non-Goals:**
- No database schema change. The six tables stay separate — a single shared
  physical table was considered and rejected (see Decisions).
- No new queue. This change touches only the six that already exist.
- No behavior change for any worker's callers, cron cadence, or ops runbook.

## Decisions

- **`BatchProcessor[C]` returns `(Stats, error)`, not a bare `error` — changed while
  migrating `embed`.** The original sketch assumed one homogeneous batch call per
  wave, succeed-or-fail as a whole. `embed`'s real shape splits a wave into two
  independent groups (open jobs to (re-)embed, closed jobs to remove), each with
  its own batch-call-then-per-item-fallback cycle — and both `embed` and
  `search-drain` also need to leave a wave claimed, untouched, on a mere call
  timeout (`skipOnTimeout`) rather than fall back per-item into what would become
  many equally slow calls. A bare `error` return can't express "I already handled
  this myself, here's the real per-outcome tally" or "I chose to do nothing" — only
  "fully succeeded" or "fully failed, please retry every item for me." Forcing
  `embed`'s dual-group shape through the binary contract would have meant treating
  a closed-group failure as a whole-wave failure, triggering RunBatch's own
  fallback over items the open-group call had *already* completed (and whose
  outbox rows were already deleted) — a double-processing bug. The richer contract
  (nil error ⇒ trust the returned `Stats` verbatim, no fallback; non-nil error ⇒
  fall back to `Processor` per item, `Stats` ignored) lets a closure that manages
  its own sub-grouping report an accurate partial result, while a simple
  single-call closure (`search-drain`) still gets RunBatch's generic fallback for
  free by returning `(Stats{}, err)` on failure.

- **`RunOptions.OnWave func(Stats)`, added while migrating `enrich`.** Discovered
  mid-implementation: `enrich`, `embed`, and `search-drain` each log a per-wave
  progress heartbeat with the *cumulative* running total (e.g. `"enrich: progress
  enriched=%d failed=%d dead=%d"`), which a bare call into `RunPool`/`RunBatch`
  would have silently dropped — an external-behavior change the proposal
  explicitly rules out. `OnWave`, called with the cumulative `Stats` after each
  wave, lets those three callers reproduce their exact log line; `applyform`,
  `adzunadesc`, and `maillink` have no such log today and simply leave it unset.

- **`RunOptions` carries no `MaxAttempts` or per-call timeout.** The brainstorming
  design doc's illustrative sketch
  (`docs/superpowers/specs/2026-08-09-outbox-runner-unification-design.md`) included
  both, but the actual `internal/outbox` implementation omits them: both govern a
  single `Processor` call's own retry/timeout behavior, which lives inside each
  caller's closure alongside its own `Store` calls (confirmed against `applyform`,
  `adzunadesc`, `embed`, `searchdrain`'s existing `RunOptions`), not in the shared
  loop. Noted here explicitly so the deviation from that earlier sketch reads as
  intentional, not an oversight.

- **Keep six separate tables; do not merge into one generic outbox table.**
  Alternative considered: a single physical table with a `queue` discriminator
  and a JSONB payload. Rejected because it would put all six queues' insert/
  delete churn through one autovacuum target and one claim index, trading
  today's natural per-table parallelism (six independent B-trees, no
  cross-queue contention) for shared bloat and hot-page contention as data
  grows — particularly between the two highest-churn queues (`search_outbox`,
  `semantic_outbox`) and the low-frequency ones. The tables are already close
  enough in shape that no migration is needed to unify the Go layer either.

- **Unify at the Go layer only, via generic type parameters
  (`RunPool[C any]`, `RunBatch[C any]`).** Alternative considered: an
  interface-based runner using `any` with local per-package adapters, closer to
  the codebase's current style (no package here uses generics yet). Rejected in
  favor of generics because this is precisely the use case type parameters were
  added to Go for — one control-flow algorithm reused across otherwise-unrelated
  claim-struct types — and the `any`-based alternative would just move the type
  assertions the compiler could otherwise catch into each caller.

- **Two run shapes, not one**, because the six workers split into two genuinely
  different concurrency models driven by the cost of what they call:
  - `RunPool[C]` — bounded-concurrency, one goroutine per claimed item. Used by
    `enrich`, `applyform`, `adzunadesc` (all per-call-expensive: LLM calls, HTTP
    fetches) and by `maillink` at `Concurrency: 1`.
  - `RunBatch[C]` — one call per wave, falling back to per-item only on a
    batch-level failure. Used by `embed`, `search-drain` (Meilisearch bulk push:
    expensive per call, nearly free per item inside the call).

  A single shape would either serialize the batch-oriented workers into slow
  one-item-at-a-time Meilisearch pushes, or force the per-item workers to
  fabricate a meaningless "batch" around calls that don't batch.

- **`RunPool` at `Concurrency <= 1` processes the wave as a plain sequential
  loop — no goroutine spawned at all — rather than a pool sized to one.**
  `internal/maillink` requires strict in-order processing within a wave (a later
  message in the same email thread must see the same wave's earlier link; see
  `internal/maillink`'s `appCache`/`ThreadLinks` comments). Go's channel
  semantics between blocked senders are not formally FIFO, so a size-1
  semaphore-gated pool would not have been a safe substitute for a real
  sequential loop.

- **`Enqueue` stays outside `internal/outbox`.** It varies too much to
  generalize usefully — `enrich` enqueues by `target_version`, `embed` by
  `target_model`, `search_outbox` is enqueued by `cmd/ingest` in the same
  transaction as the job write and is never self-enqueued by `cmd/search-drain`
  at all. Each `cmd/<worker>` keeps calling its own `Store.Enqueue` (or not)
  before invoking the runner.

- **`internal/outbox` never calls `Complete`/`Fail`/`Discard` itself.** Those
  calls stay inside each package's `Processor` closure exactly as today, so
  every existing nuance (enrich's `maxAttempts=1` override for a corrupted row,
  applyform's `ErrPostingGone` → `Discard`) is preserved without the generic
  runner having to model it. The runner only tallies whichever `Outcome`
  (`Succeeded`/`Failed`/`DeadLettered`/`Discarded`) the closure reports.

- **`RunPool`/`RunBatch` have no built-in context-cancellation early-exit.**
  Discovered mid-implementation: only two of the four `RunPool` callers
  (`applyform`, `adzunadesc`) check `ctx.Err()` after a wave and return
  `(stats, nil)` to avoid burning a large backlog's retry attempts on
  shutdown; `enrich` and `maillink` have no such check today and instead let a
  cancelled context surface as an ordinary error from the next `Claim` call.
  Baking either behavior into the shared runner would silently change the
  other pair's exit code on SIGTERM — exactly the kind of behavior change this
  change promises not to make. Resolution: `internal/outbox` stays
  cancellation-agnostic; `applyform` and `adzunadesc`'s own `Claimer` adapters
  reproduce their historical early-exit locally (check `ctx.Err()` first,
  return `(nil, nil)` instead of querying — the runner's ordinary
  empty-batch-stops-cleanly path then reproduces the exact same outcome).

- **All six workers migrate in one change, not a phased rollout.** The per-queue
  diff is small (swap an internal loop, not any external contract), and the
  queues are similar enough that a multi-week phased migration would only leave
  the codebase inconsistent for longer with no corresponding risk reduction.

## Risks / Trade-offs

- [Risk] A subtle behavior change in the shared loop (e.g. lease-timing edge
  case) would affect all six workers at once instead of just one. → Mitigation:
  `internal/outbox` gets its own unit tests covering claim-until-empty
  termination, `MaxPerRun` budget shrinking, `RunPool`'s sequential-vs-pooled
  branching, and the batch-failure-triggers-fallback path — the loop mechanics
  currently
  duplicated (and separately tested) six times move to one well-tested place
  instead of six less-scrutinized copies.
- [Risk] `internal/maillink`'s `Store.ClaimBatch(ctx, leaseSeconds, batchSize)`
  takes its two size arguments in the opposite order from every other package's
  `Claim(ctx, batch, leaseSeconds)`. → Mitigation: `Claimer[C].Claim` standardizes
  on the latter order; `maillink`'s adapter gets a two-line argument-order
  wrapper, not a signature change to the underlying `Store`.
- [Trade-off] This is the first use of Go generics in the codebase. → Accepted:
  the use case (one algorithm, many claim-struct types) is the intended one for
  type parameters, not a stylistic reach; scope is confined to
  `internal/outbox`, so it does not obligate other packages to adopt generics.

## Migration Plan

Pure internal refactor: no schema migration, no sqlc query change, no
`Complete`/`Save` signature change, no env var change, no systemd unit change, no
change to any `cmd/<name>` binary's name or external behavior. Land as a small
stack of six mechanical, independently bisectable commits (one per package
swapping its hand-rolled loop for a call into `internal/outbox`), preceded by the
`internal/outbox` package itself with its own tests. No rollback plan beyond
`git revert`, since nothing outside the Go binaries changes — a bad revert is a
redeploy, not a data migration.

## Open Questions

None outstanding — all decisions above were confirmed with the user during
brainstorming (see the linked design doc for the question-by-question
resolution, including why the single-table option and the schema-alignment
option were both explicitly rejected).
