package main

import (
	"context"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

type fakeLister struct {
	rows  []db.ListUncategorizedOpenJobTitlesRow
	calls int
}

func (f *fakeLister) ListUncategorizedOpenJobTitles(_ context.Context, p db.ListUncategorizedOpenJobTitlesParams) ([]db.ListUncategorizedOpenJobTitlesRow, error) {
	f.calls++
	var page []db.ListUncategorizedOpenJobTitlesRow
	for _, r := range f.rows {
		if r.ID > p.AfterID && len(page) < int(p.BatchSize) {
			page = append(page, r)
		}
	}
	return page, nil
}

// Pages past a full first page and keeps only the titles search keeps uncategorized.
func TestKeptUncategorizedIDsPagesAndFilters(t *testing.T) {
	f := &fakeLister{}
	for i := int64(1); i <= uncategorizedPage+2; i++ {
		f.rows = append(f.rows, db.ListUncategorizedOpenJobTitlesRow{ID: i, Title: "Barista"})
	}
	f.rows[0].Title = "General Manager"
	f.rows[uncategorizedPage+1].Title = "Vodja oddelka (m/ž)"

	got, err := keptUncategorizedIDs(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	if want := []int64{1, uncategorizedPage + 2}; !slices.Equal(got, want) {
		t.Errorf("kept = %v, want %v", got, want)
	}
	if f.calls != 2 {
		t.Errorf("pages read = %d, want 2", f.calls)
	}
}
