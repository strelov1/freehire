//go:build integration

// Integration tests for the thumbs-voting SQL: the toggle/flip upsert, the
// clear paths, and the single-target counter recompute. These are SQL behavior
// (including the Postgres rule that a data-modifying CTE's effect is invisible to
// sibling sub-statements — hence recount is its own statement run after the write)
// and can only be verified against a real Postgres. Run with:
//
//	go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// pgInt2 wraps a direction as a non-null pgtype.Int2 for UpsertJobVote's nullable
// vote column.
func pgInt2(v int16) pgtype.Int2 { return pgtype.Int2{Int16: v, Valid: true} }

func TestJobVoteToggleFlipAndCounters(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	uid := seedAPIKeyUser(t, pool, "jobvote@example.test")
	jid := insertJob(t, pool, "vote-1")

	// First up-vote: my_vote=1, counters up=1 down=0.
	if mv, err := q.UpsertJobVote(ctx, UpsertJobVoteParams{UserID: uid, JobID: jid, Vote: pgInt2(1)}); err != nil || mv != 1 {
		t.Fatalf("UpsertJobVote up: mv=%d err=%v", mv, err)
	}
	if r, err := q.RecountJobVotes(ctx, jid); err != nil || r.UpvoteCount != 1 || r.DownvoteCount != 0 {
		t.Fatalf("recount after up: %+v err=%v", r, err)
	}

	// Re-cast the same direction clears the vote: my_vote=0, counters up=0.
	if mv, err := q.UpsertJobVote(ctx, UpsertJobVoteParams{UserID: uid, JobID: jid, Vote: pgInt2(1)}); err != nil || mv != 0 {
		t.Fatalf("UpsertJobVote toggle-clear: mv=%d err=%v", mv, err)
	}
	if r, err := q.RecountJobVotes(ctx, jid); err != nil || r.UpvoteCount != 0 || r.DownvoteCount != 0 {
		t.Fatalf("recount after toggle-clear: %+v err=%v", r, err)
	}

	// Down then flip to up.
	if mv, _ := q.UpsertJobVote(ctx, UpsertJobVoteParams{UserID: uid, JobID: jid, Vote: pgInt2(-1)}); mv != -1 {
		t.Fatalf("down: mv=%d", mv)
	}
	if r, _ := q.RecountJobVotes(ctx, jid); r.UpvoteCount != 0 || r.DownvoteCount != 1 {
		t.Fatalf("recount after down: %+v", r)
	}
	if mv, _ := q.UpsertJobVote(ctx, UpsertJobVoteParams{UserID: uid, JobID: jid, Vote: pgInt2(1)}); mv != 1 {
		t.Fatalf("flip: mv=%d", mv)
	}
	if r, _ := q.RecountJobVotes(ctx, jid); r.UpvoteCount != 1 || r.DownvoteCount != 0 {
		t.Fatalf("recount after flip: %+v", r)
	}

	// Reader-facing vote lookup.
	if mv, err := q.GetJobVote(ctx, GetJobVoteParams{UserID: uid, JobID: jid}); err != nil || mv != 1 {
		t.Fatalf("GetJobVote: mv=%d err=%v", mv, err)
	}

	// Explicit clear.
	if err := q.ClearJobVote(ctx, ClearJobVoteParams{UserID: uid, JobID: jid}); err != nil {
		t.Fatalf("ClearJobVote: %v", err)
	}
	if mv, _ := q.GetJobVote(ctx, GetJobVoteParams{UserID: uid, JobID: jid}); mv != 0 {
		t.Fatalf("GetJobVote after clear: mv=%d", mv)
	}
	if r, _ := q.RecountJobVotes(ctx, jid); r.UpvoteCount != 0 || r.DownvoteCount != 0 {
		t.Fatalf("recount after clear: %+v", r)
	}
}

func TestJobVoteCountsDistinctUsers(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	jid := insertJob(t, pool, "vote-multi")
	u1 := seedAPIKeyUser(t, pool, "v1@example.test")
	u2 := seedAPIKeyUser(t, pool, "v2@example.test")

	_, _ = q.UpsertJobVote(ctx, UpsertJobVoteParams{UserID: u1, JobID: jid, Vote: pgInt2(1)})
	_, _ = q.UpsertJobVote(ctx, UpsertJobVoteParams{UserID: u2, JobID: jid, Vote: pgInt2(-1)})
	r, err := q.RecountJobVotes(ctx, jid)
	if err != nil || r.UpvoteCount != 1 || r.DownvoteCount != 1 {
		t.Fatalf("two-user counters: %+v err=%v", r, err)
	}
}

func TestCompanyVoteToggleAndCounters(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	insertCompany(t, pool, "acme", "Acme Inc")
	uid := seedAPIKeyUser(t, pool, "covote@example.test")

	if ok, err := q.CompanySlugExists(ctx, "acme"); err != nil || !ok {
		t.Fatalf("CompanySlugExists acme: ok=%v err=%v", ok, err)
	}
	if ok, _ := q.CompanySlugExists(ctx, "ghost"); ok {
		t.Fatalf("CompanySlugExists ghost should be false")
	}

	// Up-vote.
	if err := q.UpsertCompanyVote(ctx, UpsertCompanyVoteParams{UserID: uid, CompanySlug: "acme", Vote: 1}); err != nil {
		t.Fatalf("UpsertCompanyVote: %v", err)
	}
	if r, _ := q.RecountCompanyVotes(ctx, "acme"); r.UpvoteCount != 1 || r.DownvoteCount != 0 {
		t.Fatalf("recount after up: %+v", r)
	}
	if mv, _ := q.GetCompanyVote(ctx, GetCompanyVoteParams{UserID: uid, CompanySlug: "acme"}); mv != 1 {
		t.Fatalf("GetCompanyVote: %d", mv)
	}

	// Clear (delete row).
	if err := q.DeleteCompanyVote(ctx, DeleteCompanyVoteParams{UserID: uid, CompanySlug: "acme"}); err != nil {
		t.Fatalf("DeleteCompanyVote: %v", err)
	}
	if mv, _ := q.GetCompanyVote(ctx, GetCompanyVoteParams{UserID: uid, CompanySlug: "acme"}); mv != 0 {
		t.Fatalf("GetCompanyVote after delete: %d", mv)
	}
	if r, _ := q.RecountCompanyVotes(ctx, "acme"); r.UpvoteCount != 0 || r.DownvoteCount != 0 {
		t.Fatalf("recount after delete: %+v", r)
	}
}
