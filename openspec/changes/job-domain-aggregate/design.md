## Context

`Job` is the core entity but exists only as two anemic shapes: `db.Job` (the sqlc
persistence row) and `jobview.Job` (the wire model built by `FromRow(db.Job)`).
A valid job is *assembled* — normalize + derive facets + mint slugs — in three
write paths (`pipeline.normalizeJob`, `moderation` create/edit, `tg-extract`), each
duplicating the ritual by discipline. `tg-extract` has already diverged (it derives
inline, bypassing `jobderive`). Lifecycle (open/closed/reopen) and enrichment
eligibility live in raw SQL predicates. This is a live MVP ingest flow: the write
path runs every crawl, so behavior must be preserved while its guardianship moves
into an aggregate.

A key finding shapes the design: the two shapes are not the same field set.

- **Write draft** (`pipeline.Job`): fields a write path supplies and derives —
  source/external_id, title, company, location, description, posted_at, the six
  dictionary facets, slugs, and the synthetic enrichment facets.
- **Read projection** (`db.Job` as read by `jobview.FromRow`): the above *plus*
  persistence- and cross-aggregate-computed fields — `ID`, `ViewCount`,
  `AppliedCount`, `Collections`, `Enrichment` (JSONB), `EnrichedAt`,
  `EnrichmentVersion`, `CreatedAt`/`UpdatedAt`/`ClosedAt`, `CreatedBy`.

## Goals / Non-Goals

**Goals:**
- One guarded construction door: `job.New(Draft)` is the only way to build a fresh
  `Job`; it derives facets internally so all write paths are identical.
- A domain `Job` type distinct from `db.Job`, loaded/persisted through a
  `job.Repository` port with a `QueriesRepository` adapter over the shared queries.
- `jobview` projects from the domain `Job` (`FromDomain`) instead of `db.Job`,
  closing the persistence→wire leak. Wire output stays byte-equivalent.
- Lifecycle + `ShouldEnrich` become guarded aggregate methods; the three SQL close
  paths and the enrichment predicate express their decision through the aggregate.

**Non-Goals:**
- Splitting the 119-method `db.Queries` per context. The port sits on top of it.
- Rewriting the `UpsertJob`/outbox write-path transaction or moving closing into a
  single new transaction. The SQL queries stay; only the *decision* moves.
- Making `Job` a rich behavioral aggregate. It stays data-centric — the value is the
  single door and invariant location, not behavior richness.
- Changing *when* a job closes, *whether* it enriches, or *what* facets are derived.

## Decisions

### D1. One domain type, two entry points

`job.Job` is a single struct. Two ways to obtain one:
- `job.New(Draft) (Job, error)` — a *fresh* job for the write path. Runs `jobderive`
  internally; persistence-computed fields are zero (id 0, counts 0, not enriched,
  open).
- `Repository.Load(ctx, identity) (Job, error)` — a *hydrated* job with all fields
  populated from storage.

*Alternative considered:* separate `NewJob` (write) and `JobView` (read) types. Rejected
— it reintroduces the very split we are removing and doubles the mapping surface.

### D2. The aggregate boundary excludes engagement counts and collections

`ViewCount`, `AppliedCount`, and `Collections` are **not** `Job` aggregate state — they
are projections of *other* aggregates (`user_jobs`, `job_collections`). Folding them
into `job.Job` would make the aggregate own data it cannot enforce. So the domain
`Job` carries intrinsic fields + facets + lifecycle + typed `enrichment`; the
projection supplies the cross-aggregate extras:

```
jobview.FromDomain(j job.Job, x job.Extras) (jobview.Job, error)
   where job.Extras = { ViewCount, AppliedCount int32; Collections []string }
```

**Resolved:** the clean-aggregate option was chosen — `job.Job` stays free of counts
and collections; a separate `job.Extras` carrier holds them. **The carrier lives in the
`job` package, not `jobview`** — the repository's `Load` returns `(Job, Extras, error)`,
and if `Extras` lived in `jobview` the domain would import the wire layer (a layering
inversion). `jobview.FromDomain(job.Job, job.Extras)` keeps the dependency arrow
`jobview → job`. A fresh `New` has no `Extras`; only `Load` populates one (via
`extrasFromRow`). *Alternative:* put counts on `job.Job` for convenience — rejected on
aggregate-boundary grounds; it would blur the write surface with read-only fields.

### D3. Repository port mirrors the existing `jobtracking` convention

`type Repository interface { Load(...); Upsert(...); Close(...); Reopen(...) }` with
`QueriesRepository` adapting `*db.Queries`, and a `var _ Repository = (*QueriesRepository)(nil)`
compile-time assertion — the same shape `jobtracking` already ships, so the codebase
gains no new pattern, only a second instance of an established one.

### D4. The aggregate owns the *decision*, the repository owns the *persistence*

Lifecycle methods do not run SQL. `job.Close(reason)` mutates in-memory state
idempotently (sets `closedAt` if unset, preserves slug/enrichment); the repository's
`Close`/`Upsert` performs the write via the existing `CloseUnseenJobs` /
`CloseJobBySourceExternalID` queries. `ShouldEnrich()` returns `open && version < current`
— the enrichment queue keeps its SQL filter, but the canonical rule is readable on the
type and the predicate is asserted equivalent in a test. This keeps the non-goal
(no transaction rewrite) intact while giving the rule one home.

### D5. Migration order preserves behavior; `tg-extract` is switched last, intentionally changing its output

Build the aggregate + a golden-equivalence test (`FromDomain(loaded) ≡ FromRow(row)`
and `New(draft) ≡ normalizeJob`) BEFORE switching any caller. Then switch callers one
at a time under green golden tests: `pipeline` → `moderation` → `tg-extract`. For
`pipeline` and `moderation` the switch is behavior-neutral (same `jobderive` output).
For `tg-extract` the switch **changes** its facet output — that is the intended bug
fix (it stops deriving inline). Call it out: after the switch, re-ingested Telegram
jobs get dictionary-consistent facets, so a `cmd/reindex` is warranted to refresh
existing tg documents.

## Risks / Trade-offs

- **[Factory output drifts from `normalizeJob`, breaking live ingest]** → Gate every
  caller switch behind a golden test asserting `job.New(draft)` reproduces the current
  `normalizeJob` struct field-for-field before deleting the old assembly.
- **[Projection output drifts, changing the public API]** → Golden test asserting
  `FromDomain` emits byte-equivalent JSON to `FromRow` for a representative fixture
  (enriched, unenriched, closed, with/without counts) before switching read callers.
- **[`tg-extract` facet change ripples to existing rows]** → Expected and desired;
  documented as a post-switch `reindex`. Scope is only Telegram-sourced jobs.
- **[Aggregate-boundary bikeshedding on counts/collections]** → Resolved by D2; the
  `Extras` carrier keeps the boundary explicit and testable.
- **[Enrichment JSONB ↔ typed round-trip loses raw LLM values]** → The domain type
  parses `enrichment` for projection but the repository persists the JSONB verbatim on
  `Upsert` (write path never touches `enrichment`, per the existing decoupling of
  `SetJobEnrichment` from `UpsertJob`). No round-trip on the write path.

## Migration Plan

1. Add `internal/job` (type, `New`, `Repository` port + `QueriesRepository` adapter,
   lifecycle methods) with unit tests. No caller changes yet.
2. Add `jobview.FromDomain` + `Extras`; golden test vs `FromRow`. Keep `FromRow` as a
   thin shim delegating to `FromDomain` so read callers are untouched initially.
3. Switch `pipeline.normalizeJob` to `job.New`; golden test gates it. Delete the old
   inline assembly.
4. Switch `moderation` create/edit to `job.New`.
5. Switch `tg-extract` to `job.New`; drop inline derivation. Note reindex.
6. Route the three close paths + enrichment predicate through the aggregate methods.
7. Repoint read callers from `FromRow` to `FromDomain`; remove the `FromRow` shim.

**Rollback:** each step is an independent commit behind a golden test; reverting any
single step leaves the tree green because the shims (`FromRow` delegating, callers
switched one at a time) keep both paths valid until the final cleanup.

## Open Questions

- Should `job.Repository` expose a narrow `Load`-by-identity only, or also the
  batch/list loads `jobview.FromRows` feeds? Leaning narrow first (identity + upsert +
  close), leaving list projection reading `db.Job` slices through a `FromDomain`-per-row
  shim, to avoid a large read-query migration in this change.
- Does `moderation`'s edit path need `Reopen()` semantics, or is edit always on an open
  job? To confirm against `moderation.go` during task 4.
