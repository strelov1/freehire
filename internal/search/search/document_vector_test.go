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
	return skillvec.WeightsFromCounts(map[string]int64{"go": 5000, "erlang": 12})
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

// A job that LOSES its skills must clear the vector it used to carry. The indexers
// push with PUT (Meilisearch's add-or-update), which MERGES fields, so simply omitting
// `_vectors` leaves the old one in place and the posting keeps ranking by skills it no
// longer has — verified against a live engine. An explicit null is what clears it.
func TestFromJobWithNoRecognisedSkillsClearsTheVector(t *testing.T) {
	doc, err := FromJob(db.Job{ID: 1, PublicSlug: "s", Title: "Engineer", Skills: []string{"definitely-not-a-skill"}}, docWeights())
	if err != nil {
		t.Fatalf("FromJob: %v", err)
	}
	v, ok := doc.Vectors[SkillEmbedder]
	if !ok {
		t.Fatalf("no %q key: an omitted key MERGES, leaving a stale vector behind", SkillEmbedder)
	}
	if v != nil {
		t.Errorf("vector = %v, want an explicit nil so the engine clears it", v)
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Contains(b, []byte(`"_vectors":{"skills":null}`)) {
		t.Errorf("serialised as %s, want an explicit null clear", b)
	}
}

func TestFromJobWithNoSkillsClearsTheVector(t *testing.T) {
	doc, err := FromJob(db.Job{ID: 1, PublicSlug: "s", Title: "Engineer"}, docWeights())
	if err != nil {
		t.Fatalf("FromJob: %v", err)
	}
	v, ok := doc.Vectors[SkillEmbedder]
	if !ok || v != nil {
		t.Errorf("Vectors = %v, want an explicit nil clear for a job with no skills", doc.Vectors)
	}
}

// Weights being unavailable is NOT the same as a job having no skills, and the
// difference matters: "no weights" means the rarity rollup has not run or failed, and
// wiping every vector a rebuild wrote because a drain wave could not read a snapshot
// would be a self-inflicted outage of the whole sort. So an unweighted document omits
// the key entirely and leaves whatever is stored alone.
func TestFromJobWithoutWeightsLeavesTheStoredVectorAlone(t *testing.T) {
	doc, err := FromJob(db.Job{ID: 1, PublicSlug: "s", Title: "Engineer", Skills: []string{"go"}}, skillvec.Weights{})
	if err != nil {
		t.Fatalf("FromJob: %v", err)
	}
	if doc.Vectors != nil {
		t.Errorf("Vectors = %v; with no weights loaded the key must be absent, not a null clear", doc.Vectors)
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if bytes.Contains(b, []byte(`"_vectors"`)) {
		t.Errorf("serialised as %s, want no _vectors key at all", b)
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
