package handler

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

type fakeSimilarReader struct {
	source db.GetSimilarJobIDsRow
	rows   []db.Job
}

func (f fakeSimilarReader) GetSimilarJobIDs(context.Context, int64) (db.GetSimilarJobIDsRow, error) {
	return f.source, nil
}

func (f fakeSimilarReader) GetJobsByIDs(context.Context, []int64) ([]db.Job, error) {
	// Reversed, so the test also proves the nearest-first order comes from the stored ids.
	out := slices.Clone(f.rows)
	slices.Reverse(out)
	return out, nil
}

// neighbours builds one open job per country, nearest first, slugged n1, n2, ...
func neighbours(countries ...string) ([]int64, []db.Job) {
	ids := make([]int64, len(countries))
	rows := make([]db.Job, len(countries))
	for i, c := range countries {
		ids[i] = int64(i + 1)
		rows[i] = db.Job{ID: ids[i], PublicSlug: fmt.Sprintf("n%d", i+1), Title: "Engineer", Countries: []string{c}}
	}
	return ids, rows
}

func similarSlugs(t *testing.T, r fakeSimilarReader, limit int) []string {
	t.Helper()
	views, err := similarViews(context.Background(), r, 99, limit, func(row db.Job) bool { return !row.ClosedAt.Valid })
	if err != nil {
		t.Fatalf("similarViews: %v", err)
	}
	out := make([]string, len(views))
	for i, v := range views {
		out[i] = v.PublicSlug
	}
	return out
}

func TestSimilarViewsPutsTheSourceCountryFirst(t *testing.T) {
	ids, rows := neighbours("us", "de", "in", "de", "pl", "us", "de", "de")
	rows[7].ClosedAt = pgtype.Timestamptz{Valid: true} // a closed local neighbour must not jump the queue.
	r := fakeSimilarReader{source: db.GetSimilarJobIDsRow{SimilarJobIds: ids, Countries: []string{"de"}}, rows: rows}

	got := similarSlugs(t, r, 6)
	want := []string{"n2", "n4", "n7", "n1", "n3", "n5"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSimilarViewsMatchesAnySharedCountry(t *testing.T) {
	ids, rows := neighbours("us", "pl", "de")
	rows[1].Countries = []string{"pl", "fr"}
	r := fakeSimilarReader{source: db.GetSimilarJobIDsRow{SimilarJobIds: ids, Countries: []string{"fr", "de"}}, rows: rows}

	got := similarSlugs(t, r, 6)
	want := []string{"n2", "n3", "n1"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSimilarViewsKeepsTheStoredOrderWithoutASourceCountry(t *testing.T) {
	ids, rows := neighbours("us", "de", "in")
	r := fakeSimilarReader{source: db.GetSimilarJobIDsRow{SimilarJobIds: ids}, rows: rows}

	got := similarSlugs(t, r, 2)
	want := []string{"n1", "n2"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
