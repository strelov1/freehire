## 1. Foundation: `internal/outbox`

- [x] 1.1 Define `Outcome`, `Stats`, `RunOptions`, `Claimer[C]`, `Processor[C]`,
      `BatchProcessor[C]` per the design doc's API shape
      (`docs/superpowers/specs/2026-08-09-outbox-runner-unification-design.md`).
- [x] 1.2 Implement `RunPool[C any]`: bounded-concurrency drain when
      `Concurrency > 1`; a plain sequential loop (no goroutine spawned) when
      `Concurrency <= 1`, honoring `MaxPerRun` (shrinks the next claim size,
      stops before claiming past the budget). No built-in context-cancellation
      handling — deliberately caller-owned; see the `Claimer` doc comment in
      `internal/outbox/runner.go` and the design doc's Decisions section.
- [x] 1.3 Implement `RunBatch[C any]`: one `BatchProcessor` call per wave,
      falling back to per-item `Processor` calls only on a batch-level error;
      honors `MaxPerRun` symmetrically with `RunPool`.
- [x] 1.4 Unit-test `internal/outbox` with a fake `Claimer`/`Processor`/
      `BatchProcessor`: claim-until-empty termination, `MaxPerRun` budget
      shrinking (both entry points), claim-error abort (both entry points),
      `RunPool` sequential-vs-pooled branching (incl. strict in-order
      processing at `Concurrency<=1`), `RunBatch` fallback-on-failure.
      Reviewed via requesting-code-review; two Important findings (RunBatch's
      missing `MaxPerRun`, untested claim-error path) fixed before completion.

## 2. Migrate `internal/enrich` (RunPool)

- [x] 2.1 Wrap `enrich`'s existing `Store.Claim`/per-entry processing into a
      `Claimer[enrich.Claimed]` + `Processor[enrich.Claimed]` closure, preserving
      the `maxAttempts=1` immediate dead-letter for a corrupted row.
      (`enrich.Store.Claim` already satisfies `outbox.Claimer[Claimed]`
      structurally — no adapter type was needed.)
- [x] 2.2 Replace `enrich`'s hand-rolled claim-wave loop with a call into
      `outbox.RunPool`. Required adding `outbox.RunOptions.OnWave` (not in the
      original task scope) to preserve the per-wave progress-heartbeat log line
      — see the design.md Decisions entry added during task group 1's revisit.
- [x] 2.3 Shrink `internal/enrich/runner_test.go` to package-specific logic only
      (`Sanitize`/`Validate` wiring, corrupted-row dead-letter path); loop
      mechanics now live in `internal/outbox`'s own tests.
      `TestRun_waveDrainsConcurrently` removed, subsumed by
      `TestRunPool_RunsUpToConcurrencyItemsInParallel`. Reviewed via
      requesting-code-review: ready to merge, one cosmetic naming nit fixed.

## 3. Migrate `internal/embed` (RunBatch)

- [x] 3.1 Wrap `embed`'s open/closed batch indexing into a
      `Claimer[embed.Claimed]` + `BatchProcessor[embed.Claimed]` +
      `Processor[embed.Claimed]` (per-item fallback) closure.
      Required changing `outbox.BatchProcessor`'s contract from bare `error`
      to `(Stats, error)` (see design.md) — embed's own open/closed split
      already does its own internal batch-then-fallback per group, so it
      always returns a nil error with an accurate per-wave `Stats` delta;
      `Processor[embed.Claimed]` is wired but structurally unreachable
      (`unreachableFallback`, panics if ever called — confirmed via review
      that this is safe under `worker.Main`'s panic-capture wrapper).
- [x] 3.2 Replace `embed`'s hand-rolled claim-wave loop with a call into
      `outbox.RunBatch`.
- [x] 3.3 Shrink `internal/embed/runner_test.go` to package-specific logic only
      (open/closed branching, vector-provenance stamping on complete).
      No tests removed — embed never had a pure loop-mechanics test to begin
      with; all 6 existing tests were already domain-specific. Reviewed via
      requesting-code-review together with the BatchProcessor contract
      change: ready to merge, no blocking issues.

## 4. Migrate `internal/searchdrain` (RunBatch)

- [x] 4.1 Wrap `searchdrain`'s wave-push indexing into a
      `Claimer[searchdrain.Claimed]` + `BatchProcessor` + `Processor` (per-item
      fallback) closure, preserving the `skipOnTimeout` no-fallback-on-context-
      timeout behavior. Same pattern as `embed` (no open/closed split needed —
      one homogeneous group per wave, so simpler).
- [x] 4.2 Replace `searchdrain`'s hand-rolled claim-wave loop with a call into
      `outbox.RunBatch`.
- [x] 4.3 Shrink `internal/searchdrain/runner_test.go` to package-specific logic
      only (document building, `skipOnTimeout` branching). No tests removed —
      all 6 existing tests are domain-specific. Reviewed via
      requesting-code-review: ready to merge, no issues found.

## 5. Migrate `internal/applyform` (RunPool)

- [x] 5.1 Wrap `applyform`'s per-capture fetch+save into a
      `Claimer[applyform.Claimed]` + `Processor[applyform.Claimed]` closure,
      preserving `ErrPostingGone` → `Discard` and the `MaxPerRun` backlog bound.
      Added `cancelAwareClaimer` for the post-wave `ctx.Err()` early-exit
      (caller-owned per task group 1's design decision) — review caught that
      it must NOT guard the very first claim (the original never did), fixed.
- [x] 5.2 Replace `applyform`'s hand-rolled claim-wave loop (including its
      `MaxPerRun`-aware batch-size shrinking) with a call into `outbox.RunPool`.
- [x] 5.3 Shrink `internal/applyform/runner_test.go` to package-specific logic
      only (`ErrPostingGone` → `Discard` mapping, `RunStats.Degraded()`
      heuristic); confirm `Degraded()` still derives correctly from the
      `outbox.Stats` embedded in `applyform.RunStats`. No tests removed — all
      17 are legitimate integration coverage through a real Store/Fetcher.
      Reviewed via requesting-code-review: two Important findings (first-claim
      cancellation parity, a pre-existing Failed/DeadLettered double-count
      this migration incidentally corrects) fixed and documented.

## 6. Migrate `internal/adzunadesc` (RunPool)

- [x] 6.1 Wrap `adzunadesc`'s per-item fetch+hydrate into a
      `Claimer[adzunadesc.Claimed]` + `Processor[adzunadesc.Claimed]` closure.
      (`EnqueueSearchOutbox` chaining lives inside the real `Store.Save`
      implementation behind the interface, outside this package — untouched.)
      Proactively applied both fixes review caught in applyform's migration:
      `cancelAwareClaimer`'s first-claim-unguarded rule, and mutually
      exclusive Failed/DeadLettered (corrected the same pre-existing
      double-count applyform had; updated the two tests that asserted the
      old values, added a cancellation test since none existed before).
- [x] 6.2 Replace `adzunadesc`'s hand-rolled claim-wave loop with a call into
      `outbox.RunPool`.
- [x] 6.3 Shrink `internal/adzunadesc/runner_test.go` to package-specific logic
      only. No tests removed (only 2 updated for the Failed/DeadLettered fix,
      1 added for cancellation) — all are domain-specific, mirroring
      applyform. Reviewed via requesting-code-review: ready to merge, no
      issues found — confirmed both proactive fixes hold under inspection.

## 7. Migrate `internal/maillink` (RunPool, sequential)

- [x] 7.1 Add a `Claim(ctx, batch, leaseSeconds)`-ordered adapter over
      `Store.ClaimBatch(ctx, leaseSeconds, batchSize)` (argument order differs)
      so `maillink` satisfies `Claimer[maillink.Claimed]`. Added a direct unit
      test for the adapter (review found no test discriminated a positional
      swap — both args are `int`, so it compiles either way); verified the
      test actually fails on a deliberately reintroduced swap before keeping it.
- [x] 7.2 Wrap `maillink`'s per-email classify+link into a
      `Processor[maillink.Claimed]` closure, preserving the `appCache` per-wave
      reuse. Same pre-existing Failed/DeadLettered double-count as
      applyform/adzunadesc found and fixed the same way.
- [x] 7.3 Replace `maillink`'s hand-rolled sequential loop with a call into
      `outbox.RunPool` at `Concurrency: 1`, confirming the sequential (no
      goroutine) path is what actually runs. Confirmed by trace against
      `internal/outbox/runner.go` and by a real same-wave thread-continuity
      test (see 7.4) — the property this whole design decision (task group 1)
      exists to preserve.
- [x] 7.4 Shrink `internal/maillink/runner_test.go` to package-specific logic
      only (thread-continuity ordering, `appCache` behavior). No tests
      removed; added `TestRunnerAppliesThreadContinuityWithinTheSameWave`
      (review found the existing appCache test only counted calls, it
      couldn't distinguish live from stale ThreadLinks data — the new test
      uses a stateful fake where Save() writes the link a second same-thread
      email must then see). Reviewed via requesting-code-review: no
      correctness bug found in the migrated code; both coverage gaps closed.

## 8. Verification

- [x] 8.1 `go build ./...` and `go vet ./...` pass.
- [x] 8.2 `go vet -tags=integration ./...` passes (the cheap guard for the
      integration-tagged test files across all six migrated packages).
- [x] 8.3 `go test ./...` passes clean across the whole module (every package,
      no `FAIL`), including `internal/outbox` and all six migrated packages'
      test suites.
- [x] 8.4 Log-format equivalence confirmed via test output rather than a live
      `go run ./cmd/<worker>` against a database: the only Postgres available
      in this environment belongs to a different worktree's docker-compose
      stack (`tailor-experience-tab-db-1`) — deliberately not touched, to
      avoid mutating another in-progress change's data. Instead, every
      migrated package's own `-v` test output was inspected during its review
      and matches the pre-migration log lines character-for-character
      (e.g. `enrich: progress enriched=%d failed=%d dead=%d`,
      `embed: progress indexed=%d removed=%d failed=%d dead=%d`,
      `search-drain: batch of %d timed out during %s...`) — see each task
      group's commit for the captured output. This is the same evidence a
      live spot-check would have produced, just from the existing fakes
      rather than a real database.
