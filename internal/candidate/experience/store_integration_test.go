//go:build integration

// Integration test for the bank-to-profile skill sync (task 2.6 of
// unify-skills-via-experience-bank): banking an atom against a real Postgres actually
// updates the owner's search profile, and never creates one that did not exist. A fake
// ProfileSkills can prove the Store calls it; only a real userprofile.Service + Postgres
// can prove the whole path round-trips correctly. Run with:
// go test -tags=integration ./internal/candidate/experience/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package experience

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func startPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return testdb.Pool(t)
}

func insertExperienceIntegrationUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

func seedProfile(t *testing.T, pool *pgxpool.Pool, userID int64, skills []string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO user_profiles (user_id, specializations, skills) VALUES ($1, $2, $3)`,
		userID, []string{"backend"}, skills); err != nil {
		t.Fatalf("seed profile: %v", err)
	}
}

func profileSkills(t *testing.T, pool *pgxpool.Pool, userID int64) ([]string, bool) {
	t.Helper()
	var skills []string
	err := pool.QueryRow(context.Background(),
		`SELECT skills FROM user_profiles WHERE user_id = $1`, userID).Scan(&skills)
	if err != nil {
		return nil, false
	}
	return skills, true
}

func TestStoreAddAtom_SyncsSkillsIntoAnExistingProfile(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	ctx := context.Background()

	userID := insertExperienceIntegrationUser(t, pool, "bank-sync-existing@example.test")
	seedProfile(t, pool, userID, []string{"go"})

	profileSvc := userprofile.New(userprofile.NewQueriesRepository(queries))
	bank := NewStore(NewQueriesRepository(queries)).SetProfileSkills(profileSvc)

	if _, err := bank.AddAtom(ctx, userID, Atom{
		Claim:      "Migrated the cluster to Kubernetes",
		Skills:     []string{"k8s"},
		Provenance: ProvenanceManual,
	},
		AuthorCandidate,
	); err != nil {
		t.Fatalf("AddAtom: %v", err)
	}

	skills, ok := profileSkills(t, pool, userID)
	if !ok {
		t.Fatal("profile row disappeared")
	}
	found := false
	for _, s := range skills {
		if s == "kubernetes" {
			found = true
		}
	}
	if !found {
		t.Errorf("profile skills = %v, want to include the canonicalized kubernetes", skills)
	}
	if len(skills) != 2 {
		t.Errorf("profile skills = %v, want exactly go+kubernetes (existing skill untouched, no duplication)", skills)
	}
}

func TestStoreAddAtom_LeavesNoProfileBehindForAUserWithNone(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	ctx := context.Background()

	userID := insertExperienceIntegrationUser(t, pool, "bank-sync-noprofile@example.test")

	profileSvc := userprofile.New(userprofile.NewQueriesRepository(queries))
	bank := NewStore(NewQueriesRepository(queries)).SetProfileSkills(profileSvc)

	if _, err := bank.AddAtom(ctx, userID, Atom{
		Claim:      "Migrated the cluster to Kubernetes",
		Skills:     []string{"k8s"},
		Provenance: ProvenanceManual,
	},
		AuthorCandidate,
	); err != nil {
		t.Fatalf("AddAtom: %v", err)
	}

	if _, ok := profileSkills(t, pool, userID); ok {
		t.Error("a profile row was created as a side effect of banking an atom — it must not be")
	}
}

// MergeExperienceAtoms' optimistic lock, proven only against real Postgres — mirroring
// the store_test.go note that a fake cannot prove the guarantees SQL itself carries. A
// write landing directly against the keep row, in the gap between where
// Store.MergeAtoms would have read it and where it writes, must make the merge a no-op:
// neither clobbering the concurrent edit nor deleting the loser out from under it.
func TestQueriesRepositoryMergeAtoms_RefusesAStaleUpdatedAt(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	ctx := context.Background()

	userID := insertExperienceIntegrationUser(t, pool, "merge-race@example.test")
	repo := NewQueriesRepository(queries)

	// InsertAtomIfNew is the Repository's raw write — Sanitize is Store's job, and skipping
	// it (nil Metrics/Skills) would trip the NOT NULL columns, so it's done by hand here.
	keepAtom := Atom{Claim: "Cut latency", Provenance: ProvenanceManual}
	keepAtom.Sanitize()
	keep, err := repo.InsertAtomIfNew(ctx, userID, keepAtom, ClaimKey(keepAtom.Claim))
	if err != nil {
		t.Fatalf("insert keep: %v", err)
	}
	loseAtom := Atom{Claim: "Cut latency further", Provenance: ProvenanceManual}
	loseAtom.Sanitize()
	lose, err := repo.InsertAtomIfNew(ctx, userID, loseAtom, ClaimKey(loseAtom.Claim))
	if err != nil {
		t.Fatalf("insert lose: %v", err)
	}
	staleKeepUpdatedAt := keep.UpdatedAt.Time

	// A write lands directly against Postgres — standing in for a concurrent request —
	// after Store.MergeAtoms would have already read keep and before its own write.
	if _, err := pool.Exec(ctx,
		`UPDATE experience_atoms SET context = 'raced in from another request', updated_at = now() WHERE id = $1`,
		keep.ID); err != nil {
		t.Fatalf("simulate concurrent edit: %v", err)
	}

	merged := Atom{Context: "merged context", Provenance: ProvenanceManual}
	merged.Sanitize()
	_, err = repo.MergeAtoms(ctx, userID, keep.ID, lose.ID, staleKeepUpdatedAt, lose.UpdatedAt.Time, merged)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("MergeAtoms with a stale keep updated_at = %v, want pgx.ErrNoRows", err)
	}

	after, err := repo.GetAtom(ctx, keep.ID, userID)
	if err != nil {
		t.Fatalf("GetAtom keep: %v", err)
	}
	if after.Context != "raced in from another request" {
		t.Errorf("keep context = %q, want the concurrent edit preserved, not overwritten by the aborted merge", after.Context)
	}
	if _, err := repo.GetAtom(ctx, lose.ID, userID); err != nil {
		t.Errorf("GetAtom lose: %v, want the loser to survive an aborted merge", err)
	}
}

// TestQueriesRepositoryUpdateAtomKeepingProvenance_RunsAgainstPostgres exercises the rewrite
// statement for real. sqlc generates Go from SQL but does not run it, so a column omitted from
// a SET list — which is the whole point of this query — is only proved correct here.
//
// It also pins the behaviour the statement exists for: the words change, the label does not,
// and there is no read in between for a concurrent write to slip through.
func TestQueriesRepositoryUpdateAtomKeepingProvenance_RunsAgainstPostgres(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)
	userID := insertExperienceIntegrationUser(t, pool, "rewrite-keeps-provenance@example.test")

	s := NewStore(NewQueriesRepository(queries))

	// Banked as the model's own reading: the label a laundering rewrite would want replaced.
	atom, err := s.AddAtom(ctx, userID, Atom{Claim: "Ran the payments migration"}, AuthorAgent)
	if err != nil {
		t.Fatalf("AddAtom: %v", err)
	}
	if atom.Provenance != ProvenanceAgentInferred {
		t.Fatalf("seed provenance = %q, want agent_inferred", atom.Provenance)
	}

	atom.Claim = "Ran the payments migration across three regions"
	atom.Provenance = ProvenanceManual // asked for, and must be ignored
	got, err := s.UpdateAtom(ctx, atom.ID, userID, atom, AuthorRewrite)
	if err != nil {
		t.Fatalf("UpdateAtom(rewrite): %v", err)
	}
	if got.Claim != "Ran the payments migration across three regions" {
		t.Errorf("Claim = %q, want the rewritten words", got.Claim)
	}
	if got.Provenance != ProvenanceAgentInferred {
		t.Errorf("Provenance = %q, want agent_inferred left untouched by the statement", got.Provenance)
	}

	// And it is the STORED row that kept it, not just the returned value.
	reread, err := s.GetAtom(ctx, atom.ID, userID)
	if err != nil {
		t.Fatalf("GetAtom: %v", err)
	}
	if reread.Provenance != ProvenanceAgentInferred || reread.Provenance.Publishable() {
		t.Errorf("stored provenance = %q, want agent_inferred and unpublishable", reread.Provenance)
	}
}
