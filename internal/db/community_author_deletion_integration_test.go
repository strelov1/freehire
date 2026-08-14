//go:build integration

// Integration tests for what happens to community content when its author's
// account is deleted. The FK actions are the whole behaviour here, so only a real
// Postgres can verify them. Run with: go test -tags=integration ./internal/db/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// Deleting a thread's author must not delete the thread — other members' replies
// live inside it, and their content is not the departing member's to erase.
func TestDeleteUserKeepsAuthoredCommunityContent(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, "TRUNCATE threads, thread_replies, community_personas, users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	author := insertUser(t, pool, "author@example.test")
	responder := insertUser(t, pool, "responder@example.test")
	for _, p := range []struct {
		userID int64
		handle string
	}{{author, "quiet-otter"}, {responder, "brisk-heron"}} {
		if _, err := q.InsertCommunityPersona(ctx, InsertCommunityPersonaParams{UserID: p.userID, Handle: p.handle}); err != nil {
			t.Fatalf("mint persona %s: %v", p.handle, err)
		}
	}

	thread, err := q.InsertThread(ctx, InsertThreadParams{
		SubjectType: "company", SubjectRef: "acme", Title: "Interview loop", Body: "How long?",
		AuthorUserID: pgtype.Int8{Int64: author, Valid: true},
	})
	if err != nil {
		t.Fatalf("insert thread: %v", err)
	}
	authorReply, err := q.InsertThreadReply(ctx, InsertThreadReplyParams{
		ThreadID: thread.ID, AuthorUserID: author, Body: "Bumping my own question",
	})
	if err != nil {
		t.Fatalf("insert author reply: %v", err)
	}
	responderReply, err := q.InsertThreadReply(ctx, InsertThreadReplyParams{
		ThreadID: thread.ID, AuthorUserID: responder, Body: "Four rounds over two weeks",
	})
	if err != nil {
		t.Fatalf("insert responder reply: %v", err)
	}

	if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, author); err != nil {
		t.Fatalf("delete author: %v", err)
	}

	var authorID pgtype.Int8
	if err := pool.QueryRow(ctx, `SELECT author_user_id FROM threads WHERE id = $1`, thread.ID).Scan(&authorID); err != nil {
		t.Fatalf("thread after author delete: %v (want the thread to survive)", err)
	}
	if authorID.Valid {
		t.Errorf("thread author_user_id = %d, want NULL after the author is deleted", authorID.Int64)
	}

	for _, r := range []struct {
		name string
		id   int64
	}{{"responder reply", responderReply.ID}, {"author's own reply", authorReply.ID}} {
		var body string
		if err := pool.QueryRow(ctx, `SELECT body FROM thread_replies WHERE id = $1`, r.id).Scan(&body); err != nil {
			t.Errorf("%s after author delete: %v (want it to survive)", r.name, err)
		}
	}

	// The handle is the identity, and it goes with the account.
	var personas int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM community_personas WHERE user_id = $1`, author).Scan(&personas); err != nil {
		t.Fatalf("count personas: %v", err)
	}
	if personas != 0 {
		t.Errorf("personas for the deleted author = %d, want 0", personas)
	}

	// A de-authored thread must still be readable and still listed: the persona join
	// is what carries the handle, and it must not decide whether the row exists.
	read, err := q.GetCommunityThread(ctx, thread.ID)
	if err != nil {
		t.Fatalf("GetCommunityThread after author delete: %v (want the thread to still read)", err)
	}
	if read.AuthorHandle.Valid && read.AuthorHandle.String != "" {
		t.Errorf("de-authored thread handle = %q, want none", read.AuthorHandle.String)
	}
	listed, err := q.ListOpenThreadsFirst(ctx, ListOpenThreadsFirstParams{
		SubjectType: "company", SubjectRef: "acme", Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListOpenThreadsFirst: %v", err)
	}
	if len(listed) != 1 {
		t.Errorf("listed threads = %d, want the de-authored thread to still be listed", len(listed))
	}

	replies, err := q.ListThreadRepliesFirst(ctx, ListThreadRepliesFirstParams{ThreadID: thread.ID, Limit: 10})
	if err != nil {
		t.Fatalf("ListThreadRepliesFirst: %v", err)
	}
	if len(replies) != 2 {
		t.Errorf("listed replies = %d, want both replies (one de-authored)", len(replies))
	}
}
