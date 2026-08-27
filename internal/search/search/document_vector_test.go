package search

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/strelov1/freehire/internal/dict/skillvec"
	"github.com/strelov1/freehire/internal/platform/db"
)

// docWeights are rarity weights over two real dictionary slugs.
func docWeights() skillvec.Weights {
	return skillvec.WeightsFromCounts(map[string]int64{"go": 5000, "erlang": 12}, 5012)
}

func TestFromJobCarriesTheSkillVector(t *testing.T) {
	doc, err := FromJob(db.Job{ID: 1, PublicSlug: "s", Title: "Engineer", Skills: []string{"go", "erlang"}}, docWeights())
	if err != nil {
		t.Fatalf("FromJob: %v", err)
	}
	v, ok := doc.Vectors[SkillEmbedder]
	if !ok {
		t.Fatalf("FromJob set no %q vector; document vectors = %v", SkillEmbedder, doc.Vectors)
	}
	if len(v) != skillvec.Dimensions {
		t.Errorf("vector width = %d, want %d", len(v), skillvec.Dimensions)
	}
}

// TestFromJobWithoutWeightsOmitsTheVectorEntirely matters at the wire level: an empty
// vector is a document Meilisearch rejects, not one that merely sits out the ranking.
func TestFromJobWithoutWeightsOmitsTheVectorEntirely(t *testing.T) {
	doc, err := FromJob(db.Job{ID: 1, PublicSlug: "s", Title: "Engineer", Skills: []string{"go"}}, skillvec.Weights{})
	if err != nil {
		t.Fatalf("FromJob: %v", err)
	}
	if doc.Vectors != nil {
		t.Errorf("FromJob with zero weights set Vectors = %v, want nil", doc.Vectors)
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if bytes.Contains(b, []byte(`"_vectors"`)) {
		t.Error("a vector-less document still serialised a _vectors key")
	}
}

func TestFromJobWithNoRecognisedSkillsOmitsTheVector(t *testing.T) {
	doc, err := FromJob(db.Job{ID: 1, PublicSlug: "s", Title: "Engineer", Skills: []string{"definitely-not-a-skill"}}, docWeights())
	if err != nil {
		t.Fatalf("FromJob: %v", err)
	}
	if doc.Vectors != nil {
		t.Errorf("Vectors = %v, want nil for a job whose skills are all unrecognised", doc.Vectors)
	}
}

func TestFromJobWithNoSkillsOmitsTheVector(t *testing.T) {
	doc, err := FromJob(db.Job{ID: 1, PublicSlug: "s", Title: "Engineer"}, docWeights())
	if err != nil {
		t.Fatalf("FromJob: %v", err)
	}
	if doc.Vectors != nil {
		t.Errorf("Vectors = %v, want nil for a job with no skills", doc.Vectors)
	}
}

// TestFromJobVectorSerialisesUnderTheReservedKey pins the wire contract: Meilisearch
// reads document vectors from the reserved `_vectors` object, keyed by embedder name.
func TestFromJobVectorSerialisesUnderTheReservedKey(t *testing.T) {
	doc, err := FromJob(db.Job{ID: 1, PublicSlug: "s", Title: "Engineer", Skills: []string{"go"}}, docWeights())
	if err != nil {
		t.Fatalf("FromJob: %v", err)
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out struct {
		Vectors map[string][]float32 `json:"_vectors"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Vectors[SkillEmbedder]) != skillvec.Dimensions {
		t.Errorf("_vectors[%q] has %d values, want %d", SkillEmbedder, len(out.Vectors[SkillEmbedder]), skillvec.Dimensions)
	}
}
