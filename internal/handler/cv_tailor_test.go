package handler

import (
	"context"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
)

type fakeKeyMinter struct {
	got db.CreateAPIKeyParams
}

func (f *fakeKeyMinter) CreateAPIKey(_ context.Context, arg db.CreateAPIKeyParams) (db.CreateAPIKeyRow, error) {
	f.got = arg
	return db.CreateAPIKeyRow{ID: 1, Name: arg.Name, TokenPrefix: arg.TokenPrefix}, nil
}

func TestMintTailoringKey(t *testing.T) {
	f := &fakeKeyMinter{}
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)

	token, err := mintTailoringKey(context.Background(), f, 42, now)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if token == "" {
		t.Fatal("empty token")
	}
	// The persisted hash must match the returned token, so the key authenticates.
	if f.got.TokenHash != auth.HashAPIKey(token) {
		t.Errorf("stored hash does not match returned token")
	}
	if f.got.UserID != 42 {
		t.Errorf("user = %d, want 42", f.got.UserID)
	}
	// Short-lived: expiry is now + the fixed TTL.
	if !f.got.ExpiresAt.Valid || !f.got.ExpiresAt.Time.Equal(now.Add(tailoringKeyTTL)) {
		t.Errorf("expiry = %v, want %v", f.got.ExpiresAt.Time, now.Add(tailoringKeyTTL))
	}
}
