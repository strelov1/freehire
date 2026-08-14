//go:build integration

// Integration test for InsertThreadReply's cross-thread parent guard — the review
// finding: Reply()'s client-supplied parentReplyID was passed straight to
// InsertReply with no check that it belongs to threadID, and the FK alone only
// requires the parent row to exist SOMEWHERE in thread_replies, not in the same
// thread. Only a real Postgres can verify the WITH-validated INSERT actually blocks
// (and doesn't over-block) this. Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestInsertThreadReplyRejectsParentFromAnotherThread(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, "TRUNCATE threads, thread_replies, community_personas, users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	author := insertUser(t, pool, "reply-author@example.test")

	threadA, err := q.InsertThread(ctx, InsertThreadParams{
		SubjectType: "company", SubjectRef: "acme", Title: "Thread A", Body: "body",
		AuthorUserID: pgtype.Int8{Int64: author, Valid: true},
	})
	if err != nil {
		t.Fatalf("insert thread A: %v", err)
	}
	threadB, err := q.InsertThread(ctx, InsertThreadParams{
		SubjectType: "company", SubjectRef: "acme", Title: "Thread B", Body: "body",
		AuthorUserID: pgtype.Int8{Int64: author, Valid: true},
	})
	if err != nil {
		t.Fatalf("insert thread B: %v", err)
	}

	parentInA, err := q.InsertThreadReply(ctx, InsertThreadReplyParams{
		ThreadID: threadA.ID, AuthorUserID: author, Body: "top level in A",
	})
	if err != nil {
		t.Fatalf("insert top-level reply in A: %v", err)
	}

	// Same-thread nesting must still work.
	if _, err := q.InsertThreadReply(ctx, InsertThreadReplyParams{
		ThreadID: threadA.ID, ParentReplyID: pgtype.Int8{Int64: parentInA.ID, Valid: true}, AuthorUserID: author, Body: "nested in A",
	}); err != nil {
		t.Errorf("same-thread nested reply rejected: %v, want it to succeed", err)
	}

	// Cross-thread nesting (thread B's reply naming thread A's reply as parent) must
	// be rejected — pgx.ErrNoRows, since the WHERE-validated INSERT inserts zero rows.
	_, err = q.InsertThreadReply(ctx, InsertThreadReplyParams{
		ThreadID: threadB.ID, ParentReplyID: pgtype.Int8{Int64: parentInA.ID, Valid: true}, AuthorUserID: author, Body: "nested under A's reply from B",
	})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("cross-thread nested reply err = %v, want pgx.ErrNoRows", err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM thread_replies WHERE thread_id = $1`, threadB.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("thread B reply count = %d, want 0 (the rejected insert must not land a row)", count)
	}
}
