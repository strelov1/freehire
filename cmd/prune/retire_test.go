package main

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

// retireBoards flips catalog rows, so every property here is about WHICH rows it names —
// the report decides on a board, and the catalog stores one row per region of it.

// A board listed under several regions retires under all of them: the postings the
// report judged carry only the board, so a verdict about the board cannot single out one
// of its regional rows.
func TestRetireBoardsNamesEveryRegionOfABoard(t *testing.T) {
	cat := &fakeCatalog{rows: []db.ListLiveBoardsRow{
		liveBoard("whatjobs", "dead", "us"),
		liveBoard("whatjobs", "dead", "gb"),
		liveBoard("whatjobs", "alive", "us"),
	}}
	brd, err := loadBoards(context.Background(), cat)
	if err != nil {
		t.Fatalf("loadBoards: %v", err)
	}

	retired, held, err := retireBoards(context.Background(), cat, brd, []boardKey{{"whatjobs", "dead"}})
	if err != nil {
		t.Fatalf("retireBoards: %v", err)
	}
	if retired != 2 {
		t.Errorf("retired = %d, want both regional rows", retired)
	}
	if len(held) != 0 {
		t.Errorf("held = %v, want none — the provider keeps a board", held)
	}
	for _, got := range cat.retired {
		if got.Lower != "dead" {
			t.Errorf("retired %+v, want only the dead board", got)
		}
	}
}

// The one-way door: a provider with no live board is never crawled again, and the
// company-scoped rules refuse a job they cannot re-crawl — so its postings would become
// permanently un-prunable. The refusal is decided on the whole list, not discovered
// halfway through it, so nothing of that provider is retired.
func TestRetireBoardsRefusesToEmptyAProvider(t *testing.T) {
	cat := &fakeCatalog{rows: []db.ListLiveBoardsRow{
		liveBoard("tinyats", "one"),
		liveBoard("tinyats", "two"),
	}}
	brd, err := loadBoards(context.Background(), cat)
	if err != nil {
		t.Fatalf("loadBoards: %v", err)
	}

	retired, held, err := retireBoards(context.Background(), cat, brd,
		[]boardKey{{"tinyats", "one"}, {"tinyats", "two"}})
	if err != nil {
		t.Fatalf("retireBoards: %v", err)
	}
	if retired != 0 {
		t.Errorf("retired = %d, want 0 — retiring both empties the provider", retired)
	}
	if len(cat.retired) != 0 {
		t.Errorf("wrote %v, want nothing", cat.retired)
	}
	if len(held) != 1 || held[0] != "tinyats" {
		t.Errorf("held = %v, want [tinyats] — a silent refusal reads as nothing to do", held)
	}
}

// A provider that keeps a board is retired normally in the same run that holds another
// back, so one blocked provider does not stall the rest of the wave.
func TestRetireBoardsHoldsOnlyTheProviderItWouldEmpty(t *testing.T) {
	cat := &fakeCatalog{rows: []db.ListLiveBoardsRow{
		liveBoard("tinyats", "one"),
		liveBoard("greenhouse", "dead"),
		liveBoard("greenhouse", "alive"),
	}}
	brd, err := loadBoards(context.Background(), cat)
	if err != nil {
		t.Fatalf("loadBoards: %v", err)
	}

	retired, held, err := retireBoards(context.Background(), cat, brd,
		[]boardKey{{"tinyats", "one"}, {"greenhouse", "dead"}})
	if err != nil {
		t.Fatalf("retireBoards: %v", err)
	}
	if retired != 1 {
		t.Errorf("retired = %d, want 1 (greenhouse/dead only)", retired)
	}
	if len(cat.retired) != 1 || cat.retired[0].Provider != "greenhouse" || cat.retired[0].Lower != "dead" {
		t.Errorf("retired %v, want only greenhouse/dead", cat.retired)
	}
	if len(held) != 1 || held[0] != "tinyats" {
		t.Errorf("held = %v, want [tinyats]", held)
	}
}
