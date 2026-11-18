package jobview

import (
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

// The dictionary column wins; only an empty column takes the model's category, and only a
// real one (#4412). The page, the API and search all read this projection, so they agree.
func TestCategoryFallsBackToTheModelOnlyWhenTheColumnIsEmpty(t *testing.T) {
	for _, tc := range []struct {
		name string
		job  db.Job
		want string
	}{
		{"column wins", db.Job{Category: "backend", Enrichment: []byte(`{"category":"logistics"}`)}, "backend"},
		{"empty column, model files it", db.Job{Enrichment: []byte(`{"category":"logistics"}`)}, "logistics"},
		{"model declined", db.Job{Enrichment: []byte(`{"category":"other"}`)}, ""},
		{"model off-vocabulary", db.Job{Enrichment: []byte(`{"category":"ml_platform"}`)}, ""},
		{"nothing", db.Job{}, ""},
	} {
		got, err := FromRow(tc.job)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got.Enrichment.Category != tc.want {
			t.Errorf("%s: category = %q, want %q", tc.name, got.Enrichment.Category, tc.want)
		}
	}
}
