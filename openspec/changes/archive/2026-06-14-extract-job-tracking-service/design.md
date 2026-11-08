## Context

`internal/handler/user_jobs.go` implements the per-user job interactions (view/apply/save/unsave/track). The handler reaches `h.queries` (a concrete `*db.Queries`) directly and carries the use-case rules inline: slug→id resolution, the controlled-stage check, the "provide stage and/or notes" guard, the partial-update "nil leaves unchanged" semantics, the unsaved-row → zero-interaction idempotency, and the `pgtype` ↔ JSON wire mapping. Because all of this only runs inside a Fiber request, the rules cannot be unit-tested or reused without HTTP + a database.

This is the first slice of finding #2. The goal is to establish the service-layer pattern (use-case service + narrow repository interface + thin handler) on the lowest-risk surface, so the riskier auth/OAuth extraction can follow the same shape.

The codebase already has the relevant primitives: `userjob.ValidStage` (the stage vocabulary, extracted in the contract-codegen change) and the generated `*db.Queries` for `user_jobs`.

## Goals / Non-Goals

**Goals:**

- A `internal/jobtracking` package owning the five use cases and their rules, depending on a narrow `Repository` interface (not `*db.Queries`).
- Rules unit-testable against a fake `Repository` — no Fiber, no database.
- `user_jobs.go` reduced to transport: parse, authorize, call the service, render the envelope.
- Byte-identical HTTP behavior: same JSON, same status codes, same idempotency, same auth.

**Non-Goals:**

- No `AuthService` / `JobQueryService` (later slices).
- No reduction of `Register`'s parameter list (later).
- No SQL/schema change, no new query.
- No change to routes, middleware, or the `interactionResponse` wire shape.

## Decisions

### 1. Narrow `Repository` interface defined by the consumer

`internal/jobtracking` declares the interface it needs — `JobIDBySlug`, `RecordView`, `MarkApplied`, `SaveJob`, `UnsaveJob`, `TrackJob` — and a thin adapter in the same package wraps `*db.Queries` to satisfy it. *Why:* interface segregation + testability; the service depends on a role, not on sqlc. *Alternative rejected:* injecting `*db.Queries` directly — keeps the service coupled to generated code and un-fakeable.

### 2. Service returns a domain `Interaction`, not a `db.UserJob`

The service returns a plain struct `Interaction{ JobID int64; ViewedAt, SavedAt, AppliedAt *time.Time; Stage, Notes *string }` — storage-agnostic Go types, no `pgtype`. The adapter converts `db.UserJob` (pgtype) → `Interaction`; the handler converts `Interaction` → the existing `interactionResponse` JSON. *Why:* `pgtype` is a persistence detail and must not leak into a transport-agnostic service contract. *Alternative rejected:* returning `db.UserJob` — re-couples both service and handler to pgtype.

### 3. Wire format guarded by the existing integration test + a mapping unit test

Switching `pgtype.Timestamptz`/`pgtype.Text` to `*time.Time`/`*string` risks a subtle JSON-format drift. Handler **unit** tests run with `&Handler{}` and a nil DB (they only exercise pre-DB reject paths), so the success-path JSON cannot be characterized in a unit test. Two existing/added guards cover it instead:

- **End-to-end:** `internal/handler/user_jobs_integration_test.go` (`//go:build integration`, real Postgres) already decodes the success response into `{job_id, viewed_at, saved_at, applied_at}` with `*string` timestamps and asserts the save/unsave contract + 404. It must stay green (run with `go test -tags=integration ./internal/handler/`, needs Docker). This proves the wire shape end-to-end; the `*string`-decodable timestamps confirm the current format is a plain RFC3339 string, which `*time.Time` reproduces.
- **Unit:** a focused test pins the extracted mapping `Interaction → interactionResponse` JSON encoding — field names, RFC3339 timestamp strings, and `null` for absent fields — without a DB.

*Why this split:* the unit test catches format/field drift at the mapping boundary in the fast suite; the integration test is the authoritative end-to-end check. *Constraint:* the integration suite needs Docker, so it runs in CI / locally before merge, not necessarily in every dev loop.

### 4. Use-case rules move into the service; transport stays in the handler

Into the service: slug→id resolution, stage validation (`userjob.ValidStage`), the "at least one of stage/notes" guard, the partial-update semantics, and the unsave idempotency (no row → zero `Interaction{JobID}`, nil error). Stays in the handler: `BodyParser`, reading the authenticated user id from `c.Locals` (`auth.UserID`), and rendering `{"data": …}`. The service takes `userID` and the `slug` as parameters and is unaware of Fiber/auth.

### 5. Domain errors mapped at the boundary

The service returns typed sentinels — `ErrJobNotFound` (unknown slug), `ErrInvalidStage`, `ErrEmptyTrack` — instead of `fiber.NewError` or `pgx.ErrNoRows`. The handler maps them: `ErrJobNotFound` → 404, `ErrInvalidStage`/`ErrEmptyTrack` → 400. *Why:* the service must not import Fiber or leak pgx; error semantics are part of its contract. *Alternative considered:* passing `pgx.ErrNoRows` through to the central `ErrorHandler` (which maps it to 404) — rejected because it leaks the persistence error type into the use-case contract; the explicit sentinel is clearer and keeps the 404 decision in the tracking domain.

### 6. Handler holds the service, not the queries (for these endpoints)

`handler.Register` constructs `jobtracking.New(jobtracking.NewQueriesRepository(queries))` once and stores it on `Handler`. The five tracking handlers call the service. `Handler` keeps `queries` for now (other handlers still use it); only the tracking endpoints stop touching it. *Why:* scoped, surgical; the broader `Handler`/`Register` slimming is a later slice.

## Risks / Trade-offs

- **Wire-format drift from the pgtype→time mapping** → Mitigation: the Decision-3 characterization test, written and passing against the current code first (RED captured), then held green through the refactor.
- **Behavioral drift in idempotency/partial-update edge cases** → Mitigation: the rules move with dedicated service unit tests covering each (unsave-when-absent, track stage-only, notes-only, both, invalid stage, empty body, unknown slug).
- **Over-abstraction for one slice** → Mitigation: the `Repository` is deliberately minimal (only methods the service calls); no generic CRUD, no premature shared base for the future Auth/Query services — the pattern emerges from repetition, not speculation.
- **Scope creep into the rest of #2** → Mitigation: explicit Non-Goals; `Handler` keeps `*db.Queries` for untouched endpoints.

## Migration Plan

In-place refactor on a feature branch — no deploy, schema, or data migration. Sequence: (1) characterization test on current handler, (2) introduce `jobtracking` package with the interface + service + fake-based unit tests, (3) the `*db.Queries` adapter, (4) rewire the handler to the service, (5) confirm the characterization test + full suite stay green. Rollback is reverting the branch; nothing external changes.

## Open Questions

- None blocking. (Timestamp representation is resolved by Decision 3 — the characterization test dictates the exact output; the mapping conforms to it.)
