package main

import (
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/dict/skillvec"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

// A rebuild is the only path that reaches every posting, so it is the one that has to
// carry the skill vectors. A rebuild that quietly dropped them would leave the whole
// catalogue unrankable by match with nothing failing.
func TestSplitJobs_CarriesTheSkillVector(t *testing.T) {
	job := db.Job{
		ID: 1, Title: "Go Engineer", PublicSlug: "go-x", Category: "backend",
		Description: "<p>Build things.</p>", Skills: []string{"go", "erlang"},
	}
	w := skillvec.WeightsFromCounts(map[string]int64{"go": 5000, "erlang": 12})

	docs, _, err := splitJobs([]db.Job{job}, nil, nil, time.Now(), w)
	if err != nil {
		t.Fatalf("splitJobs: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("docs = %d, want 1", len(docs))
	}
	if got := len(docs[0].Vectors[search.SkillEmbedder]); got != skillvec.Dimensions {
		t.Errorf("vector width = %d, want %d", got, skillvec.Dimensions)
	}
}

// A rebuild started before the rarity rollup ever ran must still rebuild. Losing the
// match sort for a cycle is far better than not rebuilding the index at all.
func TestSplitJobs_WithoutWeightsStillBuildsDocuments(t *testing.T) {
	job := db.Job{
		ID: 1, Title: "Go Engineer", PublicSlug: "go-x", Category: "backend",
		Description: "<p>Build things.</p>", Skills: []string{"go"},
	}

	docs, _, err := splitJobs([]db.Job{job}, nil, nil, time.Now(), skillvec.Weights{})
	if err != nil {
		t.Fatalf("splitJobs: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("docs = %d, want 1", len(docs))
	}
	if docs[0].Vectors != nil {
		t.Errorf("Vectors = %v, want nil without weights", docs[0].Vectors)
	}
}
