## Why

A valid `Job` is assembled in at least three write paths — `pipeline.normalizeJob`
(ingest), `moderation` create/edit, and `tg-extract` — and each must remember to
run the same `jobderive` derivation ritual before persisting. This invariant lives
in developer discipline, not in the type system, and it has **already diverged**:
`tg-extract/store.go` derives facets inline instead of through `jobderive`, so a
Telegram-sourced job can carry facets that a board-sourced identical posting would
not. There is no single place where a `Job` guards its own invariants; the read
model (`jobview.FromRow(db.Job)`) is built straight from the sqlc persistence row,
and lifecycle rules (open/closed/reopen, `ShouldEnrich`) are scattered across raw
SQL. Introducing a `Job` domain aggregate closes the divergence class of bug and
gives the core entity one guarded door.

## What Changes

- **New `internal/job` package** holding the `Job` domain aggregate: a storage- and
  transport-agnostic type that owns its invariants.
- **Single construction door** — `job.New(Draft)` is the *only* way to build a `Job`.
  It runs `jobderive` internally, so facets are always consistent with the source
  fields. The raw `Job{...}` composite literal is no longer constructed by callers.
- **Switch all write paths to the factory** — `pipeline.normalizeJob`,
  `moderation` create + edit, and `tg-extract`'s store all go through `job.New`,
  **removing `tg-extract`'s inline derivation** and its divergence.
- **Repository port over the shared `Queries`** — a `job.Repository` interface with a
  `QueriesRepository` adapter (mirroring the existing `jobtracking` pattern) that
  loads a `db.Job` into the domain `Job` and persists it back. The 119-method
  `db.Queries` is **not** split by context; the port sits on top of it.
- **Close the persistence→wire leak** — `jobview` projects from the domain `Job`
  (`jobview.FromDomain(job.Job)`) instead of `jobview.FromRow(db.Job)`. The public
  wire shape stops depending on the sqlc row.
- **Lifecycle + enrichment rules become aggregate behavior** — `Close(reason)`,
  `Reopen()`, and `ShouldEnrich()` become guarded methods on `Job`; the three SQL
  close paths (`CloseUnseenJobs`, `CloseJobBySourceExternalID`, liveness) and the
  enrichment-eligibility predicate route their rule through the aggregate.
- **Non-goals (explicit):** no splitting of `db.Queries` per context; no rewrite of
  the `UpsertJob`/outbox write-path transaction; no change to *when* a job closes or
  *what* facets are derived — behavior is preserved, only its guardianship moves.

## Capabilities

### New Capabilities

- `job-aggregate`: The `Job` domain aggregate — a single guarded factory as the only
  construction path (deriving facets internally), a domain type loaded and persisted
  through a repository port, wire projection from the domain type, and guarded
  lifecycle/enrichment-eligibility behavior.

### Modified Capabilities

- `deterministic-facets`: strengthen the derivation guarantee from "each write path
  calls `jobderive`" (a convention) to "every write path constructs jobs through the
  aggregate factory, so facet derivation cannot diverge between sources" (a
  type-enforced invariant). This is the spec-level change that makes the `tg-extract`
  divergence unrepresentable.

## Impact

- **New code:** `internal/job/` (aggregate, factory, repository port + adapter).
- **Modified code:** `internal/pipeline` (normalizeJob → factory), `internal/moderation`
  (create/edit → factory), `cmd/tg-extract` (drop inline derive → factory),
  `internal/jobview` (FromRow → FromDomain; stops importing shape from `db.Job`
  directly), and the lifecycle/enrichment call sites that own close/eligibility rules.
- **Unchanged:** `db.Queries` surface, migrations, the outbox/enrichment transaction,
  search indexing behavior, and all observable API/wire output (projection is
  equivalent to today's `FromRow`).
- **Risk:** live MVP ingest flow — the write path is exercised every crawl. Mitigated
  by preserving behavior (factory output ≡ today's `normalizeJob` output) and by
  golden-equivalence tests before switching each caller.
