package main

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

// full builds what save holds after the full upsert ran. Built through fullWrite rather than
// by hand, so the mapping from the generated row is covered here too.
func full(inserted, changed bool) written {
	return fullWrite(db.UpsertJobRow{
		Inserted: pgtype.Bool{Bool: inserted, Valid: true},
		Changed:  changed,
	})
}

// cheap builds what save holds after the liveness refresh ran — the branch that reports
// neither inserted nor changed because the row was neither created nor edited.
func cheap() written {
	return cheapWrite(db.RefreshUnchangedJobRow{})
}

func TestNeedsIndex(t *testing.T) {
	cases := []struct {
		name string
		w    written
		want bool
	}{
		{"new posting", full(true, true), true},
		{"edited posting", full(false, true), true},
		{"liveness-only refresh", cheap(), false},
		// Still reachable through the FULL upsert: a closed posting reopening with identical
		// content, or a cities-only drift, both miss the cheap write and then report neither.
		{"full write that changed nothing", full(false, false), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := needsIndex(c.w); got != c.want {
				t.Errorf("needsIndex(%+v) = %v, want %v", c.w, got, c.want)
			}
		})
	}
}

// The cheap branch must never claim a row the index push would then dereference: needsIndex is
// what gates the only read of written.row, so false there is what makes the nil safe.
func TestCheapWriteCarriesNoRowAndNeedsNoIndex(t *testing.T) {
	w := cheap()
	if w.row != nil {
		t.Error("cheap write carries a row, want nil — the narrow RETURNING has no row to carry")
	}
	if needsIndex(w) {
		t.Error("needsIndex(cheap) = true; the index push would dereference a nil row")
	}
}

func TestNeedsRecentFeed(t *testing.T) {
	tech := pgtype.Bool{Bool: true, Valid: true}
	notTech := pgtype.Bool{Bool: false, Valid: true}
	unresolved := pgtype.Bool{}

	marked := func(w written) written {
		w.duplicateOf = pgtype.Int8{Int64: 42, Valid: true}
		return w
	}

	cases := []struct {
		name    string
		w       written
		deduped bool
		isTech  pgtype.Bool
		want    bool
	}{
		{"new IT posting", full(true, true), false, tech, true},
		{"edited IT posting", full(false, true), false, tech, true},
		{"new non-IT posting", full(true, true), false, notTech, false},
		{"new posting with unresolved tech evidence", full(true, true), false, unresolved, false},
		{"deduped role repost", full(true, true), true, tech, false},
		{"already marked duplicate", marked(full(true, true)), false, tech, false},
		{"liveness-only refresh", cheap(), false, tech, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := needsRecentFeed(c.w, c.deduped, c.isTech); got != c.want {
				t.Errorf("needsRecentFeed(%+v, %v, %+v) = %v, want %v", c.w, c.deduped, c.isTech, got, c.want)
			}
		})
	}
}

func TestClustersByRole(t *testing.T) {
	marked := func(w written) written {
		w.duplicateOf = pgtype.Int8{Int64: 42, Valid: true}
		return w
	}
	cases := []struct {
		name string
		w    written
		want bool
	}{
		// A per-city fan-out arrives as inserts, which is the whole point of the gate.
		{"new posting", full(true, true), true},
		// Re-crawls are the bulk of a pass; the batch recompute owns them, not the hot path.
		{"edited posting", full(false, true), false},
		{"liveness-only refresh", cheap(), false},
		{"full write that changed nothing", full(false, false), false},
		// A row that already knows it is a repost has nothing to ask.
		{"already marked", marked(full(true, true)), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := clustersByRole(c.w); got != c.want {
				t.Errorf("clustersByRole(%+v) = %v, want %v", c.w, got, c.want)
			}
		})
	}
}
