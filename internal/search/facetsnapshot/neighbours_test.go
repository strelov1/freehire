package facetsnapshot

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/search/search"
)

type fakeCounter struct {
	dists   map[string]map[string]int64 // skill → skills distribution under it
	filters []string
	fail    string
}

func (f *fakeCounter) FacetCounts(_ context.Context, p search.FacetParams) (search.FacetResult, error) {
	filter := fmt.Sprint(p.Filter)
	f.filters = append(f.filters, filter)
	for skill, dist := range f.dists {
		if strings.Contains(filter, `"`+skill+`"`) {
			if skill == f.fail {
				return search.FacetResult{}, errors.New("engine down")
			}
			return search.FacetResult{Facets: map[string]map[string]int64{"skills": dist}}, nil
		}
	}
	return search.FacetResult{}, nil
}

func TestNeighboursRanksDropsSelfAndCaps(t *testing.T) {
	fc := &fakeCounter{dists: map[string]map[string]int64{
		"go":   {"go": 100, "docker": 40, "kubernetes": 40, "sql": 10, "zero": 0},
		"jira": {"jira": 9, "confluence": 7},
	}}

	rows, err := Neighbours(context.Background(), fc, []string{"go", "jira"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	want := []NeighbourRow{
		{"go", "docker", 40}, {"go", "kubernetes", 40}, // tie broken by name; sql cut by the limit
		{"jira", "confluence", 7},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("rows = %v, want %v", rows, want)
	}
	if len(fc.filters) != 2 || !strings.Contains(fc.filters[0], "skills") {
		t.Fatalf("expected one skills-filtered query per skill, got %v", fc.filters)
	}
}

func TestNeighboursAbortsOnEngineError(t *testing.T) {
	fc := &fakeCounter{fail: "jira", dists: map[string]map[string]int64{
		"go": {"docker": 1}, "jira": {"confluence": 1},
	}}
	if _, err := Neighbours(context.Background(), fc, []string{"go", "jira"}, 5); err == nil {
		t.Fatal("expected the pass to fail when one query fails")
	}
}
