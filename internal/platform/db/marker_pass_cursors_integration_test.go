//go:build integration

// Integration tests for the resume point a per-company marker pass keeps between runs
// (cmd/reindex's fuzzy collapse). Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

// No row is how a first run looks, and the caller reads ErrNoRows as "start from the top";
// a save then reads back, a second save replaces it rather than adding a row, and passes do
// not see each other's points.
func TestMarkerPassCursorRoundTrip(t *testing.T) {
	ctx := context.Background()
	q := New(startPostgres(t))

	if _, err := q.GetMarkerPassCursor(ctx, "fuzzy"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("fresh read: err = %v, want pgx.ErrNoRows", err)
	}
	for _, after := range []string{"acme", "zeta", ""} {
		if err := q.SetMarkerPassCursor(ctx, SetMarkerPassCursorParams{Pass: "fuzzy", AfterCompany: after}); err != nil {
			t.Fatalf("save %q: %v", after, err)
		}
		got, err := q.GetMarkerPassCursor(ctx, "fuzzy")
		if err != nil || got != after {
			t.Fatalf("read after saving %q = %q, %v", after, got, err)
		}
	}
	if _, err := q.GetMarkerPassCursor(ctx, "other"); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("another pass's point: err = %v, want pgx.ErrNoRows", err)
	}
}
