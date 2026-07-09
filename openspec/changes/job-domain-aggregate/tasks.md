## 1. Aggregate core (no caller changes)

- [x] 1.1 Create `internal/job` package with the domain `Job` type: intrinsic fields
  (identity, source posting fields, the six dictionary facets, slugs, synthetic
  facets), lifecycle state (`closedAt`), and typed `enrichment`; exclude engagement
  counts and collections per design D2. Unit test the type's zero value and field
  invariants.
- [x] 1.2 Implement `job.Draft` (the source-agnostic write input) and
  `job.New(Draft) (Job, error)` running `jobderive` internally. Test: a draft with a
  known title/location/description yields the expected facets and slugs, and the same
  draft always yields the same `Job`.
- [x] 1.3 Add a golden-equivalence test proving `job.New(draft)` reproduces the current
  `pipeline.normalizeJob` output field-for-field for a representative fixture set
  (remote/onsite, resolved/unresolved facets, structured source signals).
- [x] 1.4 Implement guarded lifecycle + eligibility methods: `Close(at)`
  (idempotent, preserves slug/enrichment), `Reopen()`, `ShouldEnrich()` (open &&
  version < current). Unit test idempotent close, reopen clears state, and
  `ShouldEnrich` matches the `closed_at IS NULL AND version <` predicate.

## 2. Repository port + adapter

- [x] 2.1 Define the `job.Repository` port (Load by identity + Close by identity)
  and a `QueriesRepository` adapter over `*db.Queries` with a
  `var _ Repository = (*QueriesRepository)(nil)` assertion, mirroring `jobtracking`.
  (Upsert/Reopen-persistence deferred to group 4 — see 4.0 — where the content-hash
  signal and the re-ingest consumer live; the port grows there.)
- [x] 2.2 Implement `QueriesRepository.Load` via `jobFromRow` (anti-corruption map
  `db.Job`→domain, incl. enrichment JSONB decode + pgtype→domain), added query
  `GetJobBySourceExternalID`. Unit test on the mapping + testcontainer integration test
  (`//go:build integration`) for Load/Close/NotFound.
- [x] 2.3 Implement `Close` on the adapter delegating to `CloseJobBySourceExternalID`
  (idempotent 0-rows on already-closed), integration-verified.

## 3. Wire projection from the domain type

- [x] 3.1 Add `job.Extras` (in the job pkg, not jobview — avoids a domain→wire import) (ViewCount, AppliedCount, Collections) and
  `jobview.FromDomain(job.Job, Extras) (Job, error)`; move the dict-only enrichment
  override logic into the projection.
- [x] 3.2 Golden test pins `FromDomain` output against a FROZEN wire-shape oracle
  (JSON literals captured from the original FromRow), covering enriched/unenriched/
  closed/geo-hybrid — an independent oracle, not a tautology vs the shim (review fix).
- [x] 3.3 Reduce `jobview.FromRow` to a thin shim: map `db.Job` → `job.Job` + `Extras`
  and delegate to `FromDomain`, so existing read callers stay untouched for now.

## 4. Switch write paths to the factory

- [~] 4.0 DROPPED (scope call): a repository `Upsert` was NOT added. The ingest write is a
  cohesive transaction (UpsertJob + EnqueueJobEnrichment + best-effort index) that the design
  non-goal protects ("no transaction rewrite"); a partial repository.Upsert centralizes nothing
  and adds a seam. Instead `cmd/ingest/dbStore.Save` maps `job.Fields()`→`UpsertJobParams`
  inline (hashes + tx + index unchanged). Goal 1 (single door) is delivered at CONSTRUCTION
  time via `job.New`, not persistence.

- [x] 4.1 SEALED the ingest write path: `normalizeJob`→`(job.Job, error)` via `job.New`;
  deleted the `pipeline.Job` struct; `Store.Save` now takes `job.Job` (only a factory
  product persists); both loops skip `ErrInvalidDraft` (empty title/identity) instead of
  upserting junk (new test `TestRunSkipsInvalidDraft`); `dbStore.Save` maps `Fields()`→params.
  Golden 1.3 removed (became tautological post-switch; job.New is unit-tested). Full suite green.
- [x] 4.2 Switched `moderation` Create + Update to `job.New` (via a `derive` helper);
  removed the direct `jobderive` calls. Behavior delta: moderator jobs now store a
  CleanLocation'd location (consistent with ingest; no-op for tail-less locations).
  Edit path stays a plain update (no reopen needed). Tests green (fixed one unrealistic
  fixture lacking identity).
- [x] 4.3 Switched `cmd/tg-extract` Complete + CompleteLinks to `job.New` (via
  `buildParams`), REMOVING the inline derivation — fixes the real divergence (inline
  path missed jobderive's geo-restriction/usOnly logic). Invalid extracted jobs skip
  (logged) instead of persisting junk. Added `TestNew_FacetsIndependentOfWritePath`
  (spec deterministic-facets Telegram scenario). Review confirmed non-tech category
  fallback + WorkMode precedence preserved. DEPLOY NOTE: run `cmd/reindex` after deploy
  to refresh existing tg rows whose facets now differ.

## 5. Route lifecycle + enrichment through the aggregate

- [~] 5.1 SCOPED OUT (ceremony/perf): the three close paths stay set-based SQL. The
  canonical rule already lives on the aggregate (`Close`/`Reopen`, group 1); routing the
  bulk `CloseUnseenJobs` sweep through per-job `aggregate.Close()` would be N loads+writes
  vs one `UPDATE` — a perf anti-pattern the design D4 explicitly anticipated ("aggregate
  owns the decision, repository/SQL owns the performant persistence"). No behavior change
  is available here, so the churn is unjustified. The repository `Close` (group 2) is the
  by-identity home when a caller wants it.
- [x] 5.2 Pinned `ShouldEnrich()` equivalent to the SQL enqueue predicate with an
  integration test (`TestShouldEnrich_MatchesEnqueuePredicate`): seeds open-v0 / open-v1 /
  closed-v0, runs `EnqueuePendingJobs`, asserts the aggregate rule agrees with outbox
  membership per job — guards the rule and the set-based SQL from drifting apart.

## 6. Repoint reads and clean up

- [~] 6.1 SCOPED OUT (cosmetic): `jobview.FromRow` stays as a thin, clean adapter over
  `job.FromRow` → `FromDomain`. The persistence→wire leak is already closed — the projection
  LOGIC (`FromDomain`) depends only on domain types. Removing the shim would spread its
  two-line hydration across ~10 read call sites (handlers/search) for zero behavior change,
  which contradicts "no overengineering". The shim keeps read callers untouched.
- [x] 6.2 Verified: `go build`/`go vet`/`go test ./...` green (57 pkgs) + `-tags=integration`
  for the DB tests. `internal/jobview` imports `db` in ONE place only — the deliberate
  `FromRow` shim (+ `FromRows`); the projection input shape is now `job.Job`/`job.Extras`,
  not `db.Job`.
- [x] 6.3 Updated `AGENT.md`: added the `internal/job` aggregate to the layout as the single
  construction door (built only via `job.New`/repository Load), and noted `jobview` now
  projects from the aggregate via `FromDomain`.
