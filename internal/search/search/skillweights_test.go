package search

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/dict/skillvec"
	"github.com/strelov1/freehire/internal/platform/db"
)

type fakeFacetStats struct {
	rows  []db.InsightsFacetStat
	err   error
	calls int
}

func (f *fakeFacetStats) ListFacetStats(context.Context) ([]db.InsightsFacetStat, error) {
	f.calls++
	return f.rows, f.err
}

func TestLoadSkillWeightsReadsOnlyTheSkillsFacet(t *testing.T) {
	w, err := LoadSkillWeights(context.Background(), &fakeFacetStats{rows: []db.InsightsFacetStat{
		{Facet: "skills", Value: "go", Count: 5000},
		{Facet: "skills", Value: "erlang", Count: 12},
		{Facet: "countries", Value: "DE", Count: 40000},
		{Facet: "seniority", Value: "senior", Count: 30000},
	}})
	if err != nil {
		t.Fatalf("LoadSkillWeights() error = %v", err)
	}
	rare, common := w.Vector([]string{"erlang"}), w.Vector([]string{"go"})
	if rare == nil || common == nil {
		t.Fatal("LoadSkillWeights() produced weights that build no vectors")
	}
	// The non-skill facets must not reach the weighting: skillvec anchors its scale on
	// the commonest count it is given, and a country count would dwarf every skill.
	want := skillvec.WeightsFromCounts(map[string]int64{"go": 5000, "erlang": 12})
	if got, exp := w.Vector([]string{"go", "erlang"}), want.Vector([]string{"go", "erlang"}); !sameVector(got, exp) {
		t.Error("weights differ from ones built on the skill rows alone — a non-skill facet leaked into the total")
	}
}

func TestLoadSkillWeightsWithNoSkillRowsDegradesToZero(t *testing.T) {
	w, err := LoadSkillWeights(context.Background(), &fakeFacetStats{rows: []db.InsightsFacetStat{
		{Facet: "countries", Value: "DE", Count: 40000},
	}})
	if err != nil {
		t.Fatalf("LoadSkillWeights() error = %v; a snapshot without skills is a normal pre-rollup state", err)
	}
	if got := w.Vector([]string{"go"}); got != nil {
		t.Errorf("with no skill rows, Vector() = %v, want nil", got)
	}
}

func TestLoadSkillWeightsWithAnEmptySnapshotDegradesToZero(t *testing.T) {
	w, err := LoadSkillWeights(context.Background(), &fakeFacetStats{})
	if err != nil {
		t.Fatalf("LoadSkillWeights() error = %v", err)
	}
	if got := w.Vector([]string{"go"}); got != nil {
		t.Errorf("with an empty snapshot, Vector() = %v, want nil", got)
	}
}

func TestLoadSkillWeightsPropagatesTheError(t *testing.T) {
	sentinel := errors.New("boom")
	if _, err := LoadSkillWeights(context.Background(), &fakeFacetStats{err: sentinel}); !errors.Is(err, sentinel) {
		t.Errorf("LoadSkillWeights() error = %v, want it to wrap %v", err, sentinel)
	}
}

func TestLoadSkillWeightsReadsTheSnapshotOnce(t *testing.T) {
	f := &fakeFacetStats{rows: []db.InsightsFacetStat{{Facet: "skills", Value: "go", Count: 5000}}}
	if _, err := LoadSkillWeights(context.Background(), f); err != nil {
		t.Fatalf("LoadSkillWeights() error = %v", err)
	}
	if f.calls != 1 {
		t.Errorf("ListFacetStats called %d times, want 1", f.calls)
	}
}

func sameVector(a, b []float32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
