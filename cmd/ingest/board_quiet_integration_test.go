//go:build integration

// Integration tests for the quiet-board backoff (migration 0198): the fingerprint comparison in
// RecordBoardSuccess, the Go-owned pause applied from the returned streak, and its independence
// from the failure cooldown. Run with: go test -tags=integration ./cmd/ingest/
package main

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ingest/pipeline"
	"github.com/strelov1/freehire/internal/platform/db"
)

func quietOf(t *testing.T, h *boardHealth, ctx context.Context, provider, board string) time.Time {
	t.Helper()
	p, err := h.Pauses(ctx, provider, board, "")
	if err != nil {
		t.Fatalf("Pauses %s/%s: %v", provider, board, err)
	}
	return p.Quiet
}

// An unchanged listing grows the pause by QuietFor; a changed one ends it at once.
func TestBoardHealth_QuietPauseGrowsAndResets(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	h := newBoardHealth(pool)

	// The first complete crawl records the fingerprint: nothing to compare with, so no pause.
	if err := h.RecordSuccess(ctx, "greenhouse", "quiet", "", 2, true, "fp-a"); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}
	if q := quietOf(t, h, ctx, "greenhouse", "quiet"); !q.IsZero() {
		t.Fatalf("first crawl set a quiet pause %v, want none", q)
	}

	// Two unchanged crawls: streak 2, so the pause is QuietFor(2) = 8h from now.
	for i := 0; i < 2; i++ {
		if err := h.RecordSuccess(ctx, "greenhouse", "quiet", "", 0, true, "fp-a"); err != nil {
			t.Fatalf("RecordSuccess: %v", err)
		}
	}
	want, _ := pipeline.QuietFor(2)
	q := quietOf(t, h, ctx, "greenhouse", "quiet")
	if d := time.Until(q); d < want-time.Minute || d > want+time.Minute {
		t.Errorf("after 2 unchanged crawls the pause ends in %v, want ~%v", d, want)
	}

	// The listing changes: streak back to 0 and the pause cleared.
	if err := h.RecordSuccess(ctx, "greenhouse", "quiet", "", 1, true, "fp-b"); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}
	if q := quietOf(t, h, ctx, "greenhouse", "quiet"); !q.IsZero() {
		t.Errorf("a changed listing left a quiet pause %v, want none", q)
	}

	// And the next unchanged crawl starts the sequence again at QuietFor(1).
	if err := h.RecordSuccess(ctx, "greenhouse", "quiet", "", 0, true, "fp-b"); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}
	first, _ := pipeline.QuietFor(1)
	if d := time.Until(quietOf(t, h, ctx, "greenhouse", "quiet")); d < first-time.Minute || d > first+time.Minute {
		t.Errorf("after the reset the first pause is %v, want ~%v", d, first)
	}
}

// An incomplete crawl (empty fingerprint) neither grows, resets, nor overwrites anything.
func TestBoardHealth_IncompleteCrawlLeavesQuietStateAlone(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	h := newBoardHealth(pool)

	for _, fp := range []string{"fp-a", "fp-a"} {
		if err := h.RecordSuccess(ctx, "lever", "half", "", 0, true, fp); err != nil {
			t.Fatalf("RecordSuccess: %v", err)
		}
	}
	before := quietOf(t, h, ctx, "lever", "half")
	if before.IsZero() {
		t.Fatal("setup: expected a quiet pause after an unchanged crawl")
	}
	if err := h.RecordSuccess(ctx, "lever", "half", "", 0, true, ""); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}
	if after := quietOf(t, h, ctx, "lever", "half"); !after.Equal(before) {
		t.Errorf("an incomplete crawl moved the quiet pause %v -> %v", before, after)
	}
	var streak int32
	var fp string
	if err := pool.QueryRow(ctx, `SELECT quiet_streak, listing_fingerprint FROM board_health WHERE provider='lever' AND board='half'`).Scan(&streak, &fp); err != nil {
		t.Fatalf("read row: %v", err)
	}
	if streak != 1 || fp != "fp-a" {
		t.Errorf("after an incomplete crawl streak=%d fingerprint=%q, want 1 and fp-a", streak, fp)
	}
}

// The recovery probe clears cooldowns, never quiet pauses: the two live in separate columns.
func TestBoardHealth_ClearCooldownsKeepsQuietPauses(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	h := newBoardHealth(pool)

	for _, fp := range []string{"fp", "fp"} {
		if err := h.RecordSuccess(ctx, "ashby", "calm", "", 0, true, fp); err != nil {
			t.Fatalf("RecordSuccess: %v", err)
		}
	}
	for i := 0; i < 3; i++ { // a cooled board of the same provider, so there is something to clear
		if err := h.RecordFailure(ctx, "ashby", "broken", "", "boom"); err != nil {
			t.Fatalf("RecordFailure: %v", err)
		}
	}
	before := quietOf(t, h, ctx, "ashby", "calm")
	if n, err := h.ClearCooldowns(ctx, "ashby"); err != nil || n != 1 {
		t.Fatalf("ClearCooldowns = (%d, %v), want (1, nil)", n, err)
	}
	if after := quietOf(t, h, ctx, "ashby", "calm"); after.IsZero() || !after.Equal(before) {
		t.Errorf("ClearCooldowns changed a quiet pause %v -> %v", before, after)
	}
}

// SetBoardQuietUntil is CAS-guarded: a writer holding a stale streak changes nothing.
func TestBoardHealth_SetQuietUntilIsStreakGuarded(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	h := newBoardHealth(pool)
	q := db.New(pool)

	for _, fp := range []string{"fp", "fp", "fp"} { // streak 2
		if err := h.RecordSuccess(ctx, "workable", "race", "", 0, true, fp); err != nil {
			t.Fatalf("RecordSuccess: %v", err)
		}
	}
	before := quietOf(t, h, ctx, "workable", "race")
	rows, err := q.SetBoardQuietUntil(ctx, db.SetBoardQuietUntilParams{
		Provider: "workable", Board: "race", Region: "",
		QuietUntil:  pgtype.Timestamptz{Time: time.Now().Add(time.Minute), Valid: true},
		QuietStreak: 1, // stale
	})
	if err != nil || rows != 0 {
		t.Fatalf("stale SetBoardQuietUntil = (%d, %v), want (0, nil)", rows, err)
	}
	if after := quietOf(t, h, ctx, "workable", "race"); !after.Equal(before) {
		t.Errorf("a stale writer moved the pause %v -> %v", before, after)
	}
}
