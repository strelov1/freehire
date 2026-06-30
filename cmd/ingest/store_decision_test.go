package main

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
)

func TestNeedsIndex(t *testing.T) {
	row := func(inserted, changed bool) db.UpsertJobRow {
		return db.UpsertJobRow{
			Inserted: pgtype.Bool{Bool: inserted, Valid: true},
			Changed:  changed,
		}
	}
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
