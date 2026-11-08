# Outbox runner unification — design

## Motivation

The codebase has six independent Postgres-backed work queues, each with its own worker
package under `internal/`, driven from its own `cmd/<name>` binary:

| Queue table | Worker package | `cmd/` binary |
|---|---|---|
| `enrichment_outbox` | `internal/ai/enrich` | `cmd/enrich` |
| `semantic_outbox` | `internal/ai/embed` | `cmd/embed` |
| `search_outbox` | `internal/search/searchdrain` | `cmd/search-drain` |
| `apply_form_outbox` | `internal/ingest/applyform` | `cmd/capture-apply-form` |
| `adzuna_description_outbox` | `internal/ingest/adzunadesc` | `cmd/hydrate-adzuna-description` |
| `email_classification_outbox` | `internal/application/maillink` | `cmd/classify-mail` |

All six tables are already structurally identical: `id, <subject>_id, attempts, claimed_at,
failed_at, last_error, created_at`, plus a partial claimable index on `failed_at IS NULL`.
The only divergence is a per-queue staleness key (`target_version` on `enrichment_outbox`,
`target_model` on `semantic_outbox`) and one FK name (`email_id` instead of `job_id` on
`email_classification_outbox`) — both already hidden behind each package's own `Store` port.
SQL comments across the migrations confirm the pattern was copied deliberately each time
("Mirrors ClaimSemanticBatch", "Mirrors ClaimApplyFormBatch").

Each worker package independently implements the same claim-wave drain loop: claim a
lease-bounded batch with `FOR UPDATE ... SKIP LOCKED`, process it, record success/failure/
dead-letter, repeat until a claim comes back empty. That loop — not the domain logic around
it — is what this change unifies, to remove the duplicated boilerplate and let a new queue
be added by writing only its `Store` port and processing logic.

## Decisions

- **No schema changes.** The six tables stay separate. A single shared physical table was
  considered and rejected: it would put all six queues' churn (insert/delete on every
  enqueue/complete) through one autovacuum target and one claim index, trading today's
  natural per-table parallelism (six independent B-trees, six independent workers, no
  cross-queue contention) for shared bloat and hot-page contention as data grows —
  especially between the two highest-churn queues (`search_outbox`, `semantic_outbox`) and
  the low-frequency ones. The existing columns are already close enough that no alignment
  migration is needed either.
- **Unify only the Go layer.** A new `internal/platform/outbox` package holds the generic claim-wave
  runner. Every per-queue `Store`/`Indexer`/`Provider` port, `Complete`/`Save`/`Fail`/
  `Discard` signature, and piece of domain logic (LLM retry, batch/fallback boundary,
  `ErrPostingGone` handling) stays exactly where it is, in its own package.
- **Generic over `Claimed`, not over the queue table.** `internal/platform/outbox` is parameterized
  by Go type parameters (`RunPool[C any]`, `RunBatch[C any]`) on the caller's own claim
  struct (`embed.Claimed`, `applyform.Claimed`, ...). This is the first use of generics in
  the codebase; it is the intended use case for them (one control-flow algorithm reused
  across otherwise-unrelated types), not a stylistic departure.
- **Two run shapes, not one.** The six workers split into two genuinely different
  concurrency models, driven by the economics of what they call:
  - `RunPool[C]` — bounded-concurrency, one goroutine per claimed item. Fits workers whose
    cost is per-call regardless of batching (LLM calls, HTTP fetches): `enrich`,
    `applyform`, `adzunadesc`. `classify-mail` (`internal/application/maillink`) also fits `RunPool`,
    at `Concurrency: 1` — see below for why that setting needs its own guarantee, not just
    a small pool.
  - `RunBatch[C]` — one call for the whole wave, falling back to per-item processing only
    on a batch-level failure. Fits workers whose cost is per-call and nearly free per item
    inside the call (a Meilisearch bulk push): `embed`, `search-drain`.

  A single shape imposed on both would either serialize the batch-oriented workers into
  slow one-item-at-a-time Meilisearch pushes, or force the per-item workers to fabricate a
  meaningless "batch" around calls that don't batch.
- **All six queues migrate together.** The queues are similar enough (and the change small
  enough per queue — swapping an internal loop, not any external contract) that there is no
  value in a partial rollout; splitting it into an N-week phased migration would only leave
  the codebase inconsistent for longer.

## Architecture

`internal/platform/outbox` is a new, dependency-free package (no DB, no sqlc, no Meilisearch client)
holding only the loop and its bookkeeping:

```go
package outbox

// Outcome is what a Processor did with one claimed item — the caller has already made
// whatever Store call follows from it (Complete, Fail, Discard); Runner only tallies.
type Outcome int

const (
	Succeeded Outcome = iota
	Failed
	DeadLettered
	Discarded
)

// Stats is what one Run did, in the vocabulary every worker already reports in its own
// terms via a thin wrapper (e.g. embed.Stats.Indexed == outbox.Stats.Succeeded).
type Stats struct {
	Succeeded    int
	Failed       int
	DeadLettered int
	Discarded    int
}

// Claimer leases up to batch live, unleased entries — each package's existing Claim
// method already has this shape.
type Claimer[C any] interface {
	Claim(ctx context.Context, batch, leaseSeconds int) ([]C, error)
}

// Processor handles one claimed item end to end, including whatever Store.Complete /
// Store.Fail / Store.Discard call follows, and reports what happened.
type Processor[C any] func(ctx context.Context, item C) Outcome

// BatchProcessor handles a whole wave in one call (e.g. one Meilisearch bulk push).
// A nil error means every item in the wave succeeded and was already completed by the
// call; a non-nil error means the whole wave failed and Runner falls back to processing
// it item by item through Processor.
type BatchProcessor[C any] func(ctx context.Context, items []C) error

// RunOptions are the per-run knobs. MaxPerRun is 0 (unbounded) for every queue except
// applyform today; the others simply never set it, so behavior is unchanged for them.
type RunOptions struct {
	BatchSize    int
	LeaseSeconds int
	Concurrency  int // RunPool only
	MaxAttempts  int
	MaxPerRun    int // 0 = unbounded
	CallTimeout  time.Duration
}

// RunPool drains via a bounded pool when opt.Concurrency > 1. At Concurrency <= 1 it
// processes the wave as a plain sequential loop — no goroutine is spawned at all — so a
// caller that needs in-order processing (internal/application/maillink, where a later message in a
// thread must see the same wave's earlier link) gets an actual ordering guarantee, not
// just a pool sized to one: Go's channel semantics between blocked senders are not
// formally FIFO, so a size-1 semaphore pool would not have been a safe substitute.
func RunPool[C any](ctx context.Context, claimer Claimer[C], opt RunOptions, process Processor[C]) (Stats, error)
func RunBatch[C any](ctx context.Context, claimer Claimer[C], opt RunOptions, processBatch BatchProcessor[C], processOne Processor[C]) (Stats, error)
```

Both entry points share the same outer shape: claim a wave, process it (via the pool or
the batch-then-fallback strategy), accumulate `Stats`, stop when a claim returns empty or
`ctx` is cancelled, honor `MaxPerRun` by shrinking the next claim's batch size once the
budget is nearly spent (generalized from `internal/ingest/applyform`'s existing logic).

`Enqueue` is deliberately **not** part of `internal/platform/outbox`. It varies too much to
generalize usefully (`enrich` takes a `target_version`, `embed` a `target_model`,
`search_outbox` is enqueued by `cmd/ingest` in the same transaction as the job write and
is never self-enqueued by `cmd/search-drain` at all) and each `cmd/<worker>` already calls
its own `Store.Enqueue` (or doesn't) before invoking the runner. That boundary is unchanged.

## Error handling

`internal/platform/outbox` never calls `Complete`, `Fail`, or `Discard` itself — those calls happen
inside each package's `Processor` closure exactly as they do today, preserving every
existing nuance without a generic abstraction having to model it:

- `enrich`'s immediate dead-letter (`maxAttempts=1`) for a corrupted row that can never load.
- `applyform`'s `ErrPostingGone` → `Discard` (retire without retry) versus an ordinary
  transient failure → `Fail` (retry up to `MaxAttempts`).
- `embed`/`search-drain`'s batch-level failure triggering a fallback to per-item processing
  so one poison row can't sink a whole wave.

A failure claiming the next wave (the queue itself unusable — e.g. the pool is down) still
aborts the whole `Run` with an error, as it does today; a per-item failure never does.

`outbox.Stats{Failed, DeadLettered}` plugs directly into the existing
`worker.ExitCode(failed, deadLettered int) int` convention with no change to that function.
`applyform.RunStats.Degraded()` — which deliberately does not use `worker.ExitCode` because
this worker's normal operating condition includes routine third-party failures — continues
to exist as `applyform`'s own method, now derived from an embedded `outbox.Stats` plus its
own `Discarded`-as-`Gone` renaming.

## Testing

`internal/platform/outbox` gets its own `runner_test.go`, using a fake `Claimer[int]` (or similar
minimal type) with fake `Processor`/`BatchProcessor` functions. This is the ONE place that
tests the loop mechanics currently duplicated near-verbatim across all six existing
`runner_test.go` files: claim-until-empty termination, `MaxPerRun` budget shrinking,
context-cancellation exit, and the batch-failure-triggers-fallback behavior for `RunBatch`.

Each of the six existing `runner_test.go` files shrinks to testing only what's specific to
that package: `embed`'s open/closed branching and vector-provenance stamping, `enrich`'s
`Sanitize`/`Validate` wiring and the corrupted-row dead-letter path, `applyform`'s
`ErrPostingGone` → `Discard` mapping and its `Degraded()` heuristic, and so on. Their
existing `Store`/`Indexer`/`Provider` fakes are unchanged; only the outer loop they were
driving through is replaced by a call into `internal/platform/outbox`.

## Rollout

Pure internal refactor: no schema change, no sqlc query change, no `Complete`/`Save`
signature change, no env var change, no systemd unit change, no change to any `cmd/<name>`
binary's name or external behavior. Land as one PR, or a small stack of six mechanical
commits (one per package swapping its hand-rolled loop for `internal/platform/outbox`'s), each easy
to bisect independently since the packages don't depend on each other.

## Limitations

- `internal/application/maillink`'s `Store.ClaimBatch(ctx, leaseSeconds, batchSize)` takes its two
  size arguments in the opposite order from every other package's `Claim(ctx, batch,
  leaseSeconds)`. `Claimer[C].Claim` standardizes on the latter order, so `maillink`'s
  adapter needs a two-line wrapper swapping the argument order — a naming/signature detail
  caught during inventory, not a design change.
