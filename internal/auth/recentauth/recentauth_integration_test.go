//go:build integration

package recentauth

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/testdb"
)

func TestProofIsBoundToExactSessionAndSingleUse(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users(email) VALUES('recent-auth@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	store := NewStore(pool, 0)
	sessionA := bytes.Repeat([]byte{1}, 32)
	sessionB := bytes.Repeat([]byte{2}, 32)
	raw, _, err := store.Issue(ctx, userID, 1, sessionA)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Validate(ctx, raw, userID, 1, sessionA); err != nil {
		t.Fatalf("correct session rejected: %v", err)
	}
	if err = store.Validate(ctx, raw, userID, 1, sessionB); !errors.Is(err, ErrRequired) {
		t.Fatalf("other session accepted: %v", err)
	}
	if err = store.Consume(ctx, raw, userID, 1, sessionA); err != nil {
		t.Fatal(err)
	}
	if err = store.Validate(ctx, raw, userID, 1, sessionA); !errors.Is(err, ErrRequired) {
		t.Fatalf("consumed proof accepted: %v", err)
	}
}
