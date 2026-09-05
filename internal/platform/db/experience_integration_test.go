//go:build integration

// Integration tests for the two experience-bank guarantees that are SQL behavior and
// nothing else: the unique index behind InsertExperienceAtomIfNew, and the
// coalesce/nullif write in FillExperienceEmploymentBlanks. A fake repository can be
// made to agree with either one; only a real Postgres can prove them. Run with:
// go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func seedExperienceUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("seed user %s: %v", email, err)
	}
	return id
}

// The bank's core promise is that import is additive and never duplicates. That rests on
// the unique index, not on the import code — so this is where it has to be proven.
func TestExperienceAtomClaimKeyIsUniquePerOwner(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	alice := seedExperienceUser(t, pool, "atoms-alice@example.test")
	bob := seedExperienceUser(t, pool, "atoms-bob@example.test")

	const key = "cut latency 20s to 1s"
	first, err := q.InsertExperienceAtomIfNew(ctx, InsertExperienceAtomIfNewParams{
		UserID: alice, Claim: "Cut latency 20s to 1s", ClaimKey: key,
		Provenance: "cv_import", Metrics: []string{}, Skills: []string{},
	})
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}

	// The same achievement arriving again — a re-upload, or the user saying it in chat.
	_, err = q.InsertExperienceAtomIfNew(ctx, InsertExperienceAtomIfNewParams{
		UserID: alice, Claim: "cut  latency 20s to 1s.", ClaimKey: key,
		Provenance: "stated_in_chat", Metrics: []string{}, Skills: []string{},
	})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("duplicate insert = %v, want pgx.ErrNoRows (swallowed by ON CONFLICT)", err)
	}

	atoms, err := q.ListExperienceAtoms(ctx, alice)
	if err != nil {
		t.Fatalf("ListExperienceAtoms: %v", err)
	}
	if len(atoms) != 1 {
		t.Fatalf("alice has %d atoms, want 1", len(atoms))
	}
	if atoms[0].ID != first.ID || atoms[0].Provenance != "cv_import" {
		t.Errorf("the surviving atom is %v/%s, want the first one (cv_import) — a duplicate must not overwrite",
			atoms[0].ID, atoms[0].Provenance)
	}

	// The same claim for a different person is a different fact.
	if _, err := q.InsertExperienceAtomIfNew(ctx, InsertExperienceAtomIfNewParams{
		UserID: bob, Claim: "Cut latency 20s to 1s", ClaimKey: key,
		Provenance: "manual", Metrics: []string{}, Skills: []string{},
	}); err != nil {
		t.Errorf("bob's insert = %v, want success — the claim key is owner-scoped", err)
	}
}

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

// Import must fill what the bank lacks and touch nothing it already has. A CV
// re-uploaded after the user corrected a job title must not undo that correction, and a
// CV that still reads "Present" must not resurrect a role the user has ended.
func TestExperienceFillBlanksNeverOverwrites(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	alice := seedExperienceUser(t, pool, "blanks-alice@example.test")

	created, err := q.CreateExperienceEmployment(ctx, CreateExperienceEmploymentParams{
		UserID: alice, Kind: "job",
		Company: "RingCentral", Role: "Staff Engineer", // the user's own correction
		Location:        "",
		PeriodStartYear: pgtype.Int4{Int32: 2023, Valid: true}, PeriodStartMonth: pgtype.Int2{Int16: 9, Valid: true},
		// PeriodEndYear/Month left invalid (NULL) — no end recorded yet.
		IsCurrent: false, Summary: "",
		Stack: []string{"go"},
	})
	if err != nil {
		t.Fatalf("CreateExperienceEmployment: %v", err)
	}

	filled, err := q.FillExperienceEmploymentBlanks(ctx, FillExperienceEmploymentBlanksParams{
		ID: created.ID, UserID: alice,
		Company: "RingCentral", Role: "Senior Software Engineer", // what the CV says
		Location:        "USA, Remote",
		PeriodStartYear: pgtype.Int4{Int32: 2023, Valid: true}, PeriodStartMonth: pgtype.Int2{Int16: 1, Valid: true},
		PeriodEndYear: pgtype.Int4{Int32: 2024, Valid: true},
		Summary:       "Global SaaS leader in business communications",
		Stack:         []string{"kubernetes", "go"},
	})
	if err != nil {
		t.Fatalf("FillExperienceEmploymentBlanks: %v", err)
	}

	if filled.Role != "Staff Engineer" {
		t.Errorf("role = %q, want the user's correction preserved", filled.Role)
	}
	if filled.PeriodStartYear.Int32 != 2023 || filled.PeriodStartMonth.Int16 != 9 {
		t.Errorf("period_start = %d-%d, want the existing 2023-09 preserved", filled.PeriodStartYear.Int32, filled.PeriodStartMonth.Int16)
	}
	if filled.Location != "USA, Remote" {
		t.Errorf("location = %q, want the blank filled from the CV", filled.Location)
	}
	if !filled.PeriodEndYear.Valid || filled.PeriodEndYear.Int32 != 2024 || filled.PeriodEndMonth.Valid {
		t.Errorf("period_end = %+v/%+v, want year-only 2024 filled from the CV", filled.PeriodEndYear, filled.PeriodEndMonth)
	}
	if filled.Summary == "" {
		t.Error("summary is still blank, want it filled from the CV")
	}

	// Link follows the same fill-if-blank rule as location (portfolio URL on projects).
	withLink, err := q.FillExperienceEmploymentBlanks(ctx, FillExperienceEmploymentBlanksParams{
		ID: created.ID, UserID: alice,
		Company: "RingCentral", Role: "Staff Engineer",
		Location:        "USA, Remote",
		PeriodStartYear: pgtype.Int4{Int32: 2023, Valid: true}, PeriodStartMonth: pgtype.Int2{Int16: 9, Valid: true},
		PeriodEndYear: pgtype.Int4{Int32: 2024, Valid: true},
		Summary:       "Global SaaS leader in business communications",
		Stack:         []string{"go"}, Link: "https://example.test/role",
	})
	if err != nil {
		t.Fatalf("FillExperienceEmploymentBlanks link: %v", err)
	}
	if withLink.Link != "https://example.test/role" {
		t.Errorf("link = %q, want blank filled", withLink.Link)
	}
	kept, err := q.FillExperienceEmploymentBlanks(ctx, FillExperienceEmploymentBlanksParams{
		ID: created.ID, UserID: alice,
		Company: "RingCentral", Role: "Staff Engineer",
		Location:        "USA, Remote",
		PeriodStartYear: pgtype.Int4{Int32: 2023, Valid: true}, PeriodStartMonth: pgtype.Int2{Int16: 9, Valid: true},
		PeriodEndYear: pgtype.Int4{Int32: 2024, Valid: true},
		Summary:       "Global SaaS leader in business communications",
		Stack:         []string{"go"}, Link: "https://other.example",
	})
	if err != nil {
		t.Fatalf("FillExperienceEmploymentBlanks preserve link: %v", err)
	}
	if kept.Link != "https://example.test/role" {
		t.Errorf("link = %q, want existing value preserved", kept.Link)
	}
	if filled.IsCurrent {
		t.Error("is_current became true — FillExperienceEmploymentBlanks must never touch it")
	}
	// The stack unions rather than fills-if-blank: the CV adds a technology, and the one
	// already banked survives.
	if len(filled.Stack) != 2 || filled.Stack[0] != "go" || filled.Stack[1] != "kubernetes" {
		t.Errorf("stack = %q, want [go kubernetes] — unioned and sorted", filled.Stack)
	}
}

// Deleting a role takes its evidence with it, and only the owner can do it.
func TestExperienceDeleteEmploymentCascadesToAtoms(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	alice := seedExperienceUser(t, pool, "cascade-alice@example.test")
	bob := seedExperienceUser(t, pool, "cascade-bob@example.test")

	job, err := q.CreateExperienceEmployment(ctx, CreateExperienceEmploymentParams{
		UserID: alice, Kind: "job", Company: "RingCentral", Role: "SWE", Stack: []string{},
	})
	if err != nil {
		t.Fatalf("CreateExperienceEmployment: %v", err)
	}
	if _, err := q.InsertExperienceAtomIfNew(ctx, InsertExperienceAtomIfNewParams{
		UserID: alice, EmploymentID: &job.ID, Claim: "Ran the cluster", ClaimKey: "ran the cluster",
		Provenance: "manual", Metrics: []string{}, Skills: []string{"kubernetes"},
	}); err != nil {
		t.Fatalf("InsertExperienceAtomIfNew: %v", err)
	}

	n, err := q.DeleteExperienceEmployment(ctx, DeleteExperienceEmploymentParams{ID: job.ID, UserID: bob})
	if err != nil {
		t.Fatalf("DeleteExperienceEmployment(bob): %v", err)
	}
	if n != 0 {
		t.Fatalf("bob deleted %d of alice's employments, want 0", n)
	}

	n, err = q.DeleteExperienceEmployment(ctx, DeleteExperienceEmploymentParams{ID: job.ID, UserID: alice})
	if err != nil {
		t.Fatalf("DeleteExperienceEmployment(alice): %v", err)
	}
	if n != 1 {
		t.Fatalf("alice deleted %d employments, want 1", n)
	}

	atoms, err := q.ListExperienceAtoms(ctx, alice)
	if err != nil {
		t.Fatalf("ListExperienceAtoms: %v", err)
	}
	if len(atoms) != 0 {
		t.Errorf("alice has %d atoms after deleting the role, want 0", len(atoms))
	}
}

// The backfill's cost claim is a SQL CASE, not Go: a user whose structured résumé is
// current must come back WITH it (no model call), and one whose structure is stale or
// missing must come back without it (extraction needed). Getting that inverted would
// either re-extract the whole user base through the LLM or seed the bank from structures
// the app itself treats as absent.
func TestExperienceBackfillTargetsCarryOnlyCurrentStructures(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	fresh := seedExperienceUser(t, pool, "backfill-fresh@example.test")
	stale := seedExperienceUser(t, pool, "backfill-stale@example.test")
	none := seedExperienceUser(t, pool, "backfill-none@example.test")
	noCV := seedExperienceUser(t, pool, "backfill-nocv@example.test")

	const structure = `{"experience":[{"company":"RingCentral"}]}`
	// One timestamp passed to both columns. Inside a single UPDATE the SET expressions
	// read the OLD row, so `resume_structured_uploaded_at = resume_uploaded_at` would
	// copy the pre-update value (NULL) rather than the one being written.
	uploadedAt := time.Now().UTC()
	mustExec(t, pool, `UPDATE users SET resume_object_key = 'k', resume_uploaded_at = $2,
	                   resume_structured = $3::jsonb, resume_structured_uploaded_at = $2
	                   WHERE id = $1`, fresh, uploadedAt, structure)
	mustExec(t, pool, `UPDATE users SET resume_object_key = 'k', resume_uploaded_at = $2::timestamptz,
	                   resume_structured = $3::jsonb, resume_structured_uploaded_at = $2::timestamptz - interval '1 day'
	                   WHERE id = $1`, stale, uploadedAt, structure)
	mustExec(t, pool, `UPDATE users SET resume_object_key = 'k', resume_uploaded_at = $2 WHERE id = $1`, none, uploadedAt)

	rows, err := q.ListExperienceBackfillTargets(ctx, 0)
	if err != nil {
		t.Fatalf("ListExperienceBackfillTargets: %v", err)
	}

	carried := map[int64]bool{}
	for _, row := range rows {
		carried[row.ID] = len(row.CurrentStructured) > 0
	}
	if _, listed := carried[noCV]; listed {
		t.Error("a user with no stored CV was listed as a backfill target")
	}
	if !carried[fresh] {
		t.Error("the current structure was not carried — that user would cost a needless model call")
	}
	if carried[stale] {
		t.Error("a stale structure was carried — the app treats it as absent, and so must the backfill")
	}
	if carried[none] {
		t.Error("a missing structure was reported as present")
	}

	// The single-user form narrows to exactly one row.
	one, err := q.ListExperienceBackfillTargets(ctx, fresh)
	if err != nil {
		t.Fatalf("ListExperienceBackfillTargets(one): %v", err)
	}
	if len(one) != 1 || one[0].ID != fresh {
		t.Errorf("single-user query returned %d rows, want just user %d", len(one), fresh)
	}
}

// MergeExperienceAtoms must delete the loser and update the keep in one statement, so a
// crash cannot leave both a richer keep and the loser behind.
func TestMergeExperienceAtomsIsAtomic(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	alice := seedExperienceUser(t, pool, "merge-alice@example.test")

	keep, err := q.InsertExperienceAtomIfNew(ctx, InsertExperienceAtomIfNewParams{
		UserID: alice, Claim: "Built a plugin for live and batch", ClaimKey: "built a plugin for live and batch",
		Provenance: "agent_inferred", Metrics: []string{}, Skills: []string{"python"},
	})
	if err != nil {
		t.Fatalf("insert keep: %v", err)
	}
	loser, err := q.InsertExperienceAtomIfNew(ctx, InsertExperienceAtomIfNewParams{
		UserID: alice, Claim: "Built a plugin with VAD filtering", ClaimKey: "built a plugin with vad filtering",
		Context: "model profiles", Provenance: "agent_inferred",
		Metrics: []string{"VAD"}, Skills: []string{"nlp"},
	})
	if err != nil {
		t.Fatalf("insert loser: %v", err)
	}

	merged, err := q.MergeExperienceAtoms(ctx, MergeExperienceAtomsParams{
		Context: "model profiles", Metrics: []string{"VAD"}, Skills: []string{"python", "nlp"},
		Provenance: "agent_inferred", KeepID: keep.ID, UserID: alice, LoserID: loser.ID,
		KeepUpdatedAt: keep.UpdatedAt, LoserUpdatedAt: loser.UpdatedAt,
	})
	if err != nil {
		t.Fatalf("MergeExperienceAtoms: %v", err)
	}
	if uuid.UUID(merged.ID.Bytes) != keep.ID {
		t.Fatalf("kept id = %s, want %s", uuid.UUID(merged.ID.Bytes), keep.ID)
	}
	if merged.Context != "model profiles" || len(merged.Metrics) != 1 {
		t.Errorf("merged richness: context=%q metrics=%q", merged.Context, merged.Metrics)
	}
	if merged.ClaimKey != keep.ClaimKey {
		t.Errorf("claim_key changed: %q → %q", keep.ClaimKey, merged.ClaimKey)
	}

	atoms, err := q.ListExperienceAtoms(ctx, alice)
	if err != nil {
		t.Fatalf("ListExperienceAtoms: %v", err)
	}
	if len(atoms) != 1 {
		t.Fatalf("atoms = %d, want 1", len(atoms))
	}

	_, err = q.MergeExperienceAtoms(ctx, MergeExperienceAtomsParams{
		Context: "x", Metrics: []string{}, Skills: []string{}, Provenance: "manual",
		KeepID: keep.ID, UserID: alice, LoserID: loser.ID,
		KeepUpdatedAt: merged.UpdatedAt, LoserUpdatedAt: loser.UpdatedAt,
	})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("second merge with gone loser = %v, want pgx.ErrNoRows", err)
	}
}

// A keep id that does not exist (wrong id, wrong owner, or deleted between
// Store.MergeAtoms' ownership check and this call) must leave the loser untouched — the
// merge is all-or-nothing. Regression for a bug where the DELETE ran unconditionally
// regardless of whether the paired UPDATE matched anything.
func TestMergeExperienceAtomsLeavesLoserWhenKeepIsGone(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	alice := seedExperienceUser(t, pool, "merge-keepgone@example.test")

	loser, err := q.InsertExperienceAtomIfNew(ctx, InsertExperienceAtomIfNewParams{
		UserID: alice, Claim: "Untouched claim", ClaimKey: "untouched claim",
		Provenance: "agent_inferred", Metrics: []string{}, Skills: []string{},
	})
	if err != nil {
		t.Fatalf("insert loser: %v", err)
	}

	_, err = q.MergeExperienceAtoms(ctx, MergeExperienceAtomsParams{
		Context: "x", Metrics: []string{}, Skills: []string{}, Provenance: "manual",
		KeepID: uuid.New(), UserID: alice, LoserID: loser.ID,
		KeepUpdatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}, LoserUpdatedAt: loser.UpdatedAt,
	})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("merge with nonexistent keep = %v, want pgx.ErrNoRows", err)
	}

	atoms, err := q.ListExperienceAtoms(ctx, alice)
	if err != nil {
		t.Fatalf("ListExperienceAtoms: %v", err)
	}
	if len(atoms) != 1 || atoms[0].ID != loser.ID {
		t.Fatalf("atoms = %+v, want the loser still present — a failed merge must delete nothing", atoms)
	}
}

func mustExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec: %v", err)
	}
}
