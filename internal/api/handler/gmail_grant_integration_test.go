//go:build integration

package handler

import (
	"context"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

// The Gmail callback stores the token and the scopes Google reported for it together; a
// reconnect that reports no scopes keeps the ones already recorded.
func TestSaveGmailGrantRecordsScopesWithTheToken(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	var uid int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('grant@example.test') RETURNING id`).Scan(&uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	h := newInboxHandlers(db.New(pool), pool, nil, nil, "", false, "", nil)

	read := func() (string, []string) {
		t.Helper()
		var enc string
		var scopes []string
		if err := pool.QueryRow(ctx, `SELECT refresh_token_enc, scopes FROM gmail_connections WHERE user_id = $1`, uid).Scan(&enc, &scopes); err != nil {
			t.Fatalf("read connection: %v", err)
		}
		return enc, scopes
	}

	want := []string{"https://www.googleapis.com/auth/gmail.readonly", "https://www.googleapis.com/auth/calendar"}
	if err := h.saveGmailGrant(ctx, uid, "grant@gmail.com", "enc-1", want); err != nil {
		t.Fatalf("save: %v", err)
	}
	if enc, scopes := read(); enc != "enc-1" || !slices.Equal(scopes, want) {
		t.Fatalf("after connect: token %q scopes %v, want enc-1 %v", enc, scopes, want)
	}

	if err := h.saveGmailGrant(ctx, uid, "grant@gmail.com", "enc-2", nil); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	if enc, scopes := read(); enc != "enc-2" || !slices.Equal(scopes, want) {
		t.Fatalf("after reconnect without scopes: token %q scopes %v, want enc-2 %v", enc, scopes, want)
	}
}
