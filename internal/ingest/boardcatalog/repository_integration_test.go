//go:build integration

// Integration tests for the boards repository against a real Postgres: the filtered
// unique index rejects a duplicate pending/active identity while a rejected or retired
// row releases it, Activate/Retire transition status without deleting the row, and the
// per-provider/per-submitter listings return what they should.
// Run with: go test -tags=integration ./internal/ingest/boardcatalog/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package boardcatalog

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func newRepo(t *testing.T) *QueriesRepository {
	t.Helper()
	pool := testdb.Pool(t)
	return NewQueriesRepository(db.New(pool))
}

func insertUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

func TestInsertRejectsDuplicateOfALiveBoard(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	in := InsertInput{Provider: "ashby", Board: "blitzy", Company: "Blitzy"}

	first, err := NewInserter(repo, sources.Taxonomy()).Insert(ctx, in, StatusPending)
	if err != nil {
		t.Fatalf("first Insert: %v", err)
	}
	if first.Status != StatusPending {
		t.Fatalf("first Insert status = %q, want pending", first.Status)
	}

	_, err = NewInserter(repo, sources.Taxonomy()).Insert(ctx, in, StatusPending)
	if !errors.Is(err, ErrDuplicateBoard) {
		t.Fatalf("second Insert err = %v, want ErrDuplicateBoard", err)
	}
}

// The two spellings of one iCIMS board — a bare slug and the full host — address the same
// site, which the unique index cannot see: it folds case, and the FORM fold is Go. One
// Inserter must catch the pair even when BOTH arrive inside the same run, which is what a
// harvest does: it reads the live set once, so a second spelling is only ever refused
// because the first one it wrote went into that set.
//
// This is the case a live set read once and never updated would miss, and the cost of
// missing it is not a wasted crawl: external_id is namespaced by the literal board
// string, so both spellings store the same postings under one company_slug, and the
// unseen sweep closes whichever a run did not refresh.
func TestInserterCatchesASecondSpellingWithinOneRun(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	ins := NewInserter(repo, sources.Taxonomy())

	if _, err := ins.Insert(ctx, InsertInput{
		Provider: "icims", Board: "vet", Company: "Vet",
	}, StatusPending); err != nil {
		t.Fatalf("first Insert: %v", err)
	}

	_, err := ins.Insert(ctx, InsertInput{
		Provider: "icims", Board: "careers-vet.icims.com", Company: "Vet Again",
	}, StatusPending)
	if !errors.Is(err, ErrDuplicateBoard) {
		t.Fatalf("second spelling err = %v, want ErrDuplicateBoard", err)
	}

	// And the refusal is about that board, not about the provider: an unrelated iCIMS
	// board still goes in through the same Inserter.
	if _, err := ins.Insert(ctx, InsertInput{
		Provider: "icims", Board: "other", Company: "Other",
	}, StatusPending); err != nil {
		t.Fatalf("unrelated board Insert: %v", err)
	}
}

// A rejected row (a validation failure) does not claim the identity: correcting the
// mistake and resubmitting must succeed, not collide with the earlier bad attempt.
func TestInsertAcceptsResubmissionAfterRejection(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	bad := InsertInput{Provider: "no-such-provider", Board: "acme", Company: "Acme"}

	rejected, err := NewInserter(repo, sources.Taxonomy()).Insert(ctx, bad, StatusPending)
	if err != nil {
		t.Fatalf("Insert with unknown provider: %v (want stored as rejected, not an error)", err)
	}
	if rejected.Status != StatusRejected || rejected.RejectedReason == "" {
		t.Fatalf("rejected = %+v, want status=rejected with a reason", rejected)
	}

	good := InsertInput{Provider: "greenhouse", Board: "acme", Company: "Acme"}
	reopened, err := NewInserter(repo, sources.Taxonomy()).Insert(ctx, good, StatusPending)
	if err != nil {
		t.Fatalf("resubmission after rejection: %v, want it recorded", err)
	}
	if reopened.Status != StatusPending {
		t.Errorf("reopened.Status = %q, want pending", reopened.Status)
	}
}

// Likewise a retired board's identity is free for a fresh submission.
func TestInsertAcceptsResubmissionAfterRetirement(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	in := InsertInput{Provider: "lever", Board: "dormant", Company: "Dormant Co"}

	first, err := NewInserter(repo, sources.Taxonomy()).Insert(ctx, in, StatusActive)
	if err != nil {
		t.Fatalf("first Insert: %v", err)
	}
	if found, err := repo.Retire(ctx, first.Provider, first.Board, first.Region); err != nil || !found {
		t.Fatalf("Retire = %v,%v, want found", found, err)
	}

	again, err := NewInserter(repo, sources.Taxonomy()).Insert(ctx, in, StatusActive)
	if err != nil {
		t.Fatalf("resubmission after retirement: %v, want it recorded", err)
	}
	if again.ID == first.ID || again.Status != StatusActive {
		t.Errorf("again = %+v, want a new active row", again)
	}
}

func TestRenameCorrectsAPlaceholderCompany(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	in := InsertInput{Provider: "greenhouse", Board: "acme-corp", Company: PlaceholderCompany("acme-corp")}
	b, err := NewInserter(repo, sources.Taxonomy()).Insert(ctx, in, StatusPending)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if b.Company != "Acme Corp" {
		t.Fatalf("seeded placeholder company = %q, want %q", b.Company, "Acme Corp")
	}

	found, err := repo.Rename(ctx, b.Provider, b.Board, b.Region, "Acme Corporation Inc.")
	if err != nil || !found {
		t.Fatalf("Rename = %v,%v, want found", found, err)
	}

	rows, err := repo.ListActiveForProvider(ctx, b.Provider)
	if err != nil {
		t.Fatalf("ListActiveForProvider: %v", err)
	}
	if len(rows) != 1 || rows[0].Company != "Acme Corporation Inc." {
		t.Fatalf("rows = %+v, want the renamed company", rows)
	}
}

func TestRenameReportsNotFoundForARetiredBoard(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	b, err := NewInserter(repo, sources.Taxonomy()).Insert(ctx, InsertInput{Provider: "greenhouse", Board: "gone", Company: "Gone"}, StatusActive)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if _, err := repo.Retire(ctx, b.Provider, b.Board, b.Region); err != nil {
		t.Fatalf("Retire: %v", err)
	}

	found, err := repo.Rename(ctx, b.Provider, b.Board, b.Region, "New Name")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if found {
		t.Error("Rename on a retired board reported found=true, want false")
	}
}

func TestActivateTransitionsPendingToActive(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	in := InsertInput{Provider: "workable", Board: "acme", Company: "Acme"}
	b, err := NewInserter(repo, sources.Taxonomy()).Insert(ctx, in, StatusPending)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	found, err := repo.Activate(ctx, b.Provider, b.Board, b.Region)
	if err != nil || !found {
		t.Fatalf("Activate = %v,%v, want found", found, err)
	}

	rows, err := repo.ListActiveForProvider(ctx, b.Provider)
	if err != nil {
		t.Fatalf("ListActiveForProvider: %v", err)
	}
	if len(rows) != 1 || rows[0].Status != StatusActive || rows[0].ActivatedAt == nil {
		t.Fatalf("rows = %+v, want one active row with ActivatedAt set", rows)
	}
}

func TestActivateIsANoOpWhenNoPendingBoardMatches(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()

	found, err := repo.Activate(ctx, "greenhouse", "no-such-board", "")
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if found {
		t.Error("Activate on a nonexistent board reported found=true")
	}
}

func TestListActiveForProviderExcludesRejectedAndRetired(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()

	if _, err := NewInserter(repo, sources.Taxonomy()).Insert(ctx, InsertInput{Provider: "gem", Board: "a", Company: "A"}, StatusActive); err != nil {
		t.Fatalf("insert active: %v", err)
	}
	if _, err := NewInserter(repo, sources.Taxonomy()).Insert(ctx, InsertInput{Provider: "gem", Board: "b", Company: "B"}, StatusPending); err != nil {
		t.Fatalf("insert pending: %v", err)
	}
	if _, err := NewInserter(repo, sources.Taxonomy()).Insert(ctx, InsertInput{Provider: "gem", Board: "c", Company: "C"}, StatusActive); err != nil {
		t.Fatalf("insert to-retire: %v", err)
	}
	if _, err := repo.Retire(ctx, "gem", "c", ""); err != nil {
		t.Fatalf("retire: %v", err)
	}
	if _, err := NewInserter(repo, sources.Taxonomy()).Insert(ctx, InsertInput{Provider: "no-such-provider", Board: "d", Company: "D"}, StatusActive); err != nil {
		t.Fatalf("insert rejected: %v", err)
	}

	rows, err := repo.ListActiveForProvider(ctx, "gem")
	if err != nil {
		t.Fatalf("ListActiveForProvider: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2 (only the pending and active gem boards): %+v", len(rows), rows)
	}
}

func TestListBySubmitterReturnsOnlyThatUsersBoards(t *testing.T) {
	pool := testdb.Pool(t)
	repo := NewQueriesRepository(db.New(pool))
	ctx := context.Background()
	alice := insertUser(t, pool, "alice@example.test")
	bob := insertUser(t, pool, "bob@example.test")

	if _, err := NewInserter(repo, sources.Taxonomy()).Insert(ctx, InsertInput{Provider: "recruitee", Board: "a", Company: "A", SubmittedBy: &alice}, StatusPending); err != nil {
		t.Fatalf("insert for alice: %v", err)
	}
	if _, err := NewInserter(repo, sources.Taxonomy()).Insert(ctx, InsertInput{Provider: "recruitee", Board: "b", Company: "B", SubmittedBy: &bob}, StatusPending); err != nil {
		t.Fatalf("insert for bob: %v", err)
	}

	rows, err := repo.ListBySubmitter(ctx, alice)
	if err != nil {
		t.Fatalf("ListBySubmitter: %v", err)
	}
	if len(rows) != 1 || rows[0].Board != "a" {
		t.Fatalf("rows = %+v, want exactly alice's board", rows)
	}
}
