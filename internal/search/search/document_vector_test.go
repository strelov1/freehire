package search

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/strelov1/freehire/internal/dict/skillvec"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestFromJobCarriesTheSkillVector(t *testing.T) {
	doc, err := FromJob(db.Job{ID: 1, PublicSlug: "s", Title: "Engineer", Skills: []string{"go", "erlang"}})
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
	doc, err := FromJob(db.Job{ID: 1, PublicSlug: "s", Title: "Engineer", Skills: []string{"definitely-not-a-skill"}})
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
	doc, err := FromJob(db.Job{ID: 1, PublicSlug: "s", Title: "Engineer"})
	if err != nil {
		t.Fatalf("FromJob: %v", err)
	}
	v, ok := doc.Vectors[SkillEmbedder]
	if !ok || v != nil {
		t.Errorf("Vectors = %v, want an explicit nil clear for a job with no skills", doc.Vectors)
	}
}

// TestFromJobVectorSerialisesUnderTheReservedKey pins the wire contract: Meilisearch
// reads document vectors from the reserved `_vectors` object, keyed by embedder name.
func TestFromJobVectorSerialisesUnderTheReservedKey(t *testing.T) {
	doc, err := FromJob(db.Job{ID: 1, PublicSlug: "s", Title: "Engineer", Skills: []string{"go"}})
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

// The two sides must not be swapped. A document built with the PROFILE constructor
// would carry no ballast, so it would rank as if it asked for nothing — the exact
// defect the ballast exists to fix, reintroduced silently. Assert the stored vector
// carries it.
func TestFromJobUsesTheJobSideConstructor(t *testing.T) {
	doc, err := FromJob(db.Job{ID: 1, PublicSlug: "s", Title: "Engineer", Skills: []string{"go", "docker"}})
	if err != nil {
		t.Fatalf("FromJob: %v", err)
	}
	stored := doc.Vectors[SkillEmbedder]
	want := skillvec.JobVector([]string{"go", "docker"})
	if len(stored) != len(want) {
		t.Fatalf("vector width = %d, want %d", len(stored), len(want))
	}
	for i := range want {
		if stored[i] != want[i] {
			t.Fatalf("document vector differs from JobVector at position %d — is it built with ProfileVector?", i)
		}
	}
}
