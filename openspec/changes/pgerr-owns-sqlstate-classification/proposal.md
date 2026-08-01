## Why

`internal/pgerr`'s package doc calls it "the single home for the SQLSTATE constants the
repositories and the central error handler share". That was not true: `internal/worker` declared
`XX001` itself and repeated the `errors.As(*pgconn.PgError)` unwrap, three lines below a doc that
scopes `worker` to "shared bootstrap and run-outcome plumbing".

The consequence the split predicts had already happened. `internal/enrich` and `internal/embed`
imported `internal/worker` for **nothing but** `IsCorruptedRow` — so two domain packages
transitively depended on config, database and observability in order to classify one SQLSTATE.

## What Changes

- `pgerr.IsDataCorrupted` joins `IsUniqueViolation` / `IsForeignKeyViolation`, with the `XX001`
  constant. Recognizing the condition moves; **the policy stays in `worker`** — deciding that a
  corrupted row is skipped rather than surfaced is the resilient scan's decision, not the
  taxonomy's.
- `worker.IsCorruptedRow` and `corruptDataSQLState` are deleted. `resilient.go` calls the shared
  predicate at its two sites and stops importing `pgconn` entirely.
- `internal/enrich` and `internal/embed` stop importing `internal/worker`.
- **The doc's claim becomes a test.** `TestOnlyThisPackageUnwrapsAPgError` walks the repo and
  fails on any non-test file outside `internal/pgerr` that mentions `pgconn.PgError` — a package
  that unwraps one is classifying, whatever it names the function. It refuses to pass if the walk
  scanned too few files, so a broken walk cannot read as a clean repo.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

(none) — no requirement-level behaviour change. The same errors are classified the same way;
`tasks.md` is the real artifact and the change archives with `--skip-specs`.

## Impact

- `internal/pgerr` (+ a new test file), `internal/worker/resilient.go` and its test,
  `internal/enrich/runner.go`, `internal/embed/runner.go`.
- `internal/pipeline` keeps its `internal/worker` import — it uses `worker.Heartbeat`, shared
  run-time plumbing rather than a leaked taxonomy, so the rule is about the classification and
  not about the dependency in general.
