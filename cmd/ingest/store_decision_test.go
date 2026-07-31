package main

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
)

func row(inserted, changed bool) db.UpsertJobRow {
	return db.UpsertJobRow{
		Inserted: pgtype.Bool{Bool: inserted, Valid: true},
		Changed:  changed,
	}
}

func TestNeedsIndex(t *testing.T) {
	cases := []struct {
		name string
		row  db.UpsertJobRow
		want bool
	}{
		{"new posting", row(true, true), true},
		{"edited posting", row(false, true), true},
		{"last-seen-only refresh", row(false, false), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := needsIndex(c.row); got != c.want {
				t.Errorf("needsIndex(%+v) = %v, want %v", c.row, got, c.want)
			}
		})
	}
}

func TestClustersByRole(t *testing.T) {
	marked := func(r db.UpsertJobRow) db.UpsertJobRow {
		r.Job.DuplicateOf = pgtype.Int8{Int64: 42, Valid: true}
		return r
	}
	cases := []struct {
		name string
		row  db.UpsertJobRow
		want bool
	}{
		// A per-city fan-out arrives as inserts, which is the whole point of the gate.
		{"new posting", row(true, true), true},
		// Re-crawls are the bulk of a pass; the batch recompute owns them, not the hot path.
		{"edited posting", row(false, true), false},
		{"last-seen-only refresh", row(false, false), false},
		// A row that already knows it is a repost has nothing to ask.
		{"already marked", marked(row(true, true)), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := clustersByRole(c.row); got != c.want {
				t.Errorf("clustersByRole(%+v) = %v, want %v", c.row, got, c.want)
			}
		})
	}
}
