# Provenance-Upgrade-On-Conflict Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix `InsertExperienceAtomIfNew` so a claim first banked with provenance
`agent_inferred` can be upgraded to `stated_in_chat`/`manual`/`cv_import` by a later
call, instead of being permanently stuck at `can_write_cv: false`.

**Architecture:** One SQL change (`ON CONFLICT DO NOTHING` → a conditional `DO UPDATE`
that only fires when the existing row is `agent_inferred` and the new call isn't),
regenerated through sqlc. No Go code changes — `Store.AddAtom`
(`internal/candidate/experience/store.go:178-194`) already returns success on any row and
`ErrAlreadyBanked` on `pgx.ErrNoRows`; that logic is correct for both the old and new
SQL behavior unchanged.

**Tech Stack:** Go, PostgreSQL, sqlc v1.31.1 (via `make sqlc`, Docker).

## Global Constraints

- Never hand-edit `internal/platform/db/*.go` — it is generated. Edit
  `internal/platform/db/queries/*.sql`, run `make sqlc`, commit the regenerated file
  (`internal/platform/db/AGENTS.md`).
- This is a query change, not a schema change — no new migration file.
- Run `go vet -tags=integration ./...` before pushing (compiles every
  `//go:build integration` file across the module; `go test ./...` alone does
  not catch a broken integration test).
- The fix must not change behavior for any conflict that isn't specifically
  "existing row is `agent_inferred`, new call is not" — every other dedup
  case (confirmed vs confirmed, confirmed vs agent_inferred, agent_inferred
  vs agent_inferred) keeps today's "first write wins, duplicate swallowed"
  behavior exactly.

---

### Task 1: Upgrade-on-conflict for `InsertExperienceAtomIfNew`

**Files:**
- Modify: `internal/platform/db/queries/experience.sql:84-93` (`InsertExperienceAtomIfNew`)
- Regenerate (via `make sqlc`, do not hand-edit): `internal/platform/db/experience.sql.go`
- Test: `internal/platform/db/experience_integration_test.go` (add new test functions after
  `TestExperienceAtomClaimKeyIsUniquePerOwner`, which ends at line 75)

**Interfaces:**
- Consumes: `q.InsertExperienceAtomIfNew(ctx, InsertExperienceAtomIfNewParams{...})
  (ExperienceAtom, error)` — unchanged signature, from `internal/platform/db/experience.sql.go`.
  `InsertExperienceAtomIfNewParams` fields used: `UserID int64`, `EmploymentID
  *uuid.UUID`, `Claim string`, `ClaimKey string`, `Context string`, `Metrics
  []string`, `Skills []string`, `Provenance string`, `SourceRef string`.
- Produces: nothing new is consumed by other tasks — this is the terminal fix. The
  existing `Store.AddAtom` (`internal/candidate/experience/store.go`) and every assistant tool
  built on it (`internal/api/handler/assistant_experience_tools.go`) start working
  correctly against this query with zero changes on their side.

- [ ] **Step 1: Write the failing integration tests**

Add these three tests to `internal/platform/db/experience_integration_test.go`, right after
`TestExperienceAtomClaimKeyIsUniquePerOwner` (after its closing `}` on line 75):

```go
// The honest wall's actual failure mode in production: a claim first recorded as
// agent_inferred (the model's paraphrase, unconfirmed) must become writable once a
// later call carries a genuinely different provenance — the ON CONFLICT must upgrade
// it in place rather than swallowing the second insert and leaving the claim stuck.
func TestExperienceAtomClaimKeyUpgradesFromAgentInferred(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	alice := seedExperienceUser(t, pool, "upgrade-alice@example.test")

	const key = "built reelmente.app with react and next.js"
	first, err := q.InsertExperienceAtomIfNew(ctx, InsertExperienceAtomIfNewParams{
		UserID: alice, Claim: "Built Reelmente.app with React and Next.js", ClaimKey: key,
		Provenance: "agent_inferred", Metrics: []string{}, Skills: []string{},
	})
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if first.Provenance != "agent_inferred" {
		t.Fatalf("first.Provenance = %q, want agent_inferred", first.Provenance)
	}

	// The candidate has now confirmed it verbatim in chat — the retry carries a real
	// provenance upgrade.
	upgraded, err := q.InsertExperienceAtomIfNew(ctx, InsertExperienceAtomIfNewParams{
		UserID: alice, Claim: "built reelmente.app with react and next.js.", ClaimKey: key,
		Provenance: "stated_in_chat", Metrics: []string{}, Skills: []string{},
	})
	if err != nil {
		t.Fatalf("upgrade insert = %v, want success (the atom should upgrade in place)", err)
	}
	if upgraded.ID != first.ID {
		t.Errorf("upgraded.ID = %v, want the SAME id as the original atom (%v)", upgraded.ID, first.ID)
	}
	if upgraded.Provenance != "stated_in_chat" {
		t.Errorf("upgraded.Provenance = %q, want stated_in_chat", upgraded.Provenance)
	}

	atoms, err := q.ListExperienceAtoms(ctx, alice)
	if err != nil {
		t.Fatalf("ListExperienceAtoms: %v", err)
	}
	if len(atoms) != 1 {
		t.Fatalf("alice has %d atoms after the upgrade, want 1 (no duplicate row)", len(atoms))
	}
	if atoms[0].Provenance != "stated_in_chat" {
		t.Errorf("stored provenance = %q, want stated_in_chat", atoms[0].Provenance)
	}
}

// A confirmed atom must never be downgraded by a later, weaker call — the upgrade path
// is one-directional.
func TestExperienceAtomClaimKeyNeverDowngradesFromConfirmed(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	alice := seedExperienceUser(t, pool, "nodowngrade-alice@example.test")

	const key = "led the postgres migration"
	first, err := q.InsertExperienceAtomIfNew(ctx, InsertExperienceAtomIfNewParams{
		UserID: alice, Claim: "Led the Postgres migration", ClaimKey: key,
		Provenance: "manual", Metrics: []string{}, Skills: []string{},
	})
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}

	// A later, unconfirmed paraphrase of the SAME claim must not touch it.
	_, err = q.InsertExperienceAtomIfNew(ctx, InsertExperienceAtomIfNewParams{
		UserID: alice, Claim: "led the postgres migration", ClaimKey: key,
		Provenance: "agent_inferred", Metrics: []string{}, Skills: []string{},
	})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("second insert = %v, want pgx.ErrNoRows (a confirmed atom must not be touched)", err)
	}

	got, err := q.GetExperienceAtom(ctx, GetExperienceAtomParams{ID: first.ID, UserID: alice})
	if err != nil {
		t.Fatalf("GetExperienceAtom: %v", err)
	}
	if got.Provenance != "manual" {
		t.Errorf("provenance = %q, want manual (unchanged)", got.Provenance)
	}
}

// Two unconfirmed attempts at the same claim leave it exactly as unconfirmed as
// before — this is the "still can't write" case, not the upgrade case, and must keep
// reporting ErrAlreadyBanked (via pgx.ErrNoRows) so Store.AddAtom's existing mapping
// is unchanged.
func TestExperienceAtomClaimKeyStaysUnconfirmedAcrossRepeatedAgentInferred(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	alice := seedExperienceUser(t, pool, "stillunconfirmed-alice@example.test")

	const key = "shipped the onboarding flow"
	if _, err := q.InsertExperienceAtomIfNew(ctx, InsertExperienceAtomIfNewParams{
		UserID: alice, Claim: "Shipped the onboarding flow", ClaimKey: key,
		Provenance: "agent_inferred", Metrics: []string{}, Skills: []string{},
	}); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	_, err := q.InsertExperienceAtomIfNew(ctx, InsertExperienceAtomIfNewParams{
		UserID: alice, Claim: "shipped the onboarding flow.", ClaimKey: key,
		Provenance: "agent_inferred", Metrics: []string{}, Skills: []string{},
	})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("second insert = %v, want pgx.ErrNoRows (still no genuine upgrade)", err)
	}
}
```

- [ ] **Step 2: Run the new tests to verify they fail**

Run: `go test -tags=integration ./internal/platform/db/ -run TestExperienceAtomClaimKeyUpgradesFromAgentInferred -v`

Expected: FAIL — under the current `ON CONFLICT DO NOTHING`, the "upgrade insert"
call returns `pgx.ErrNoRows` instead of succeeding, so the test fails at `t.Fatalf("upgrade insert = %v, want success...")`.

(`TestExperienceAtomClaimKeyNeverDowngradesFromConfirmed` and
`TestExperienceAtomClaimKeyStaysUnconfirmedAcrossRepeatedAgentInferred` already pass
under the current query — they lock in behavior the fix must not break. Confirm this
with `go test -tags=integration ./internal/platform/db/ -run TestExperienceAtomClaimKey -v`
before touching the SQL, so a later regression is unambiguous.)

- [ ] **Step 3: Edit the SQL query**

In `internal/platform/db/queries/experience.sql`, replace the `InsertExperienceAtomIfNew`
query (lines 84-93) with:

```sql
-- name: InsertExperienceAtomIfNew :one
-- The only insert. A claim already banked with a stronger provenance than
-- agent_inferred is never touched — ON CONFLICT DO NOTHING behavior for every case
-- except one: a claim first recorded as agent_inferred (the model's unconfirmed
-- paraphrase) is upgraded in place when a later call carries a real provenance,
-- because the candidate confirming it afterward must actually unstick the write —
-- otherwise that exact claim text stays permanently un-writable to a CV. The WHERE
-- guards both non-upgrade cases: a confirmed atom is never downgraded, and two
-- agent_inferred attempts at the same claim leave it exactly as unconfirmed as
-- before. Returns no row when there is genuinely nothing to change, which callers
-- report as ErrAlreadyBanked rather than an error.
INSERT INTO experience_atoms (user_id, employment_id, claim, claim_key, context, metrics, skills, provenance, source_ref)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (user_id, claim_key) DO UPDATE
SET provenance = EXCLUDED.provenance,
    source_ref = EXCLUDED.source_ref,
    context = EXCLUDED.context,
    updated_at = now()
WHERE experience_atoms.provenance = 'agent_inferred'
  AND EXCLUDED.provenance != 'agent_inferred'
RETURNING id, user_id, employment_id, claim, claim_key, context, metrics, skills, provenance, source_ref, created_at, updated_at;
```

- [ ] **Step 4: Regenerate sqlc**

Run: `make sqlc`

This runs sqlc via Docker and rewrites `internal/platform/db/experience.sql.go`. Confirm the
diff touches only the `InsertExperienceAtomIfNew` function body/SQL constant — no
other query in `experience.sql` should change.

- [ ] **Step 5: Run all three tests to verify they pass**

Run: `go test -tags=integration ./internal/platform/db/ -run TestExperienceAtomClaimKey -v`

Expected: PASS for all of:
- `TestExperienceAtomClaimKeyIsUniquePerOwner` (pre-existing, must still pass unchanged)
- `TestExperienceAtomClaimKeyUpgradesFromAgentInferred` (new)
- `TestExperienceAtomClaimKeyNeverDowngradesFromConfirmed` (new)
- `TestExperienceAtomClaimKeyStaysUnconfirmedAcrossRepeatedAgentInferred` (new)

- [ ] **Step 6: Run the full integration vet gate and the plain test suite**

Run, in order:

```bash
go vet -tags=integration ./...
go build ./...
go vet ./...
go test ./...
```

Expected: all four succeed with no output beyond normal build/test noise. The last
two confirm `internal/candidate/experience/store_test.go`'s fake-repo-based tests
(`TestStoreAddAtomReportsAnAlreadyBankedClaim` etc.) are unaffected — they exercise
`Store.AddAtom`'s Go logic against an in-memory fake that never changes, per the
comment at `internal/candidate/experience/store_test.go:17-21`.

- [ ] **Step 7: Commit**

```bash
git add internal/platform/db/queries/experience.sql internal/platform/db/experience.sql.go internal/platform/db/experience_integration_test.go
git commit -m "fix(experience): upgrade agent_inferred provenance on conflict instead of discarding it"
```
