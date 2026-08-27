package search

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/dict/skillvec"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestFromJob_RolesDerivedButIndexOnly(t *testing.T) {
	// Composite from resolved seniority+category.
	doc, err := FromJob(db.Job{ID: 1, PublicSlug: "s", Seniority: "senior", Category: "backend", Title: "Senior Backend Engineer"}, skillvec.Weights{})
	if err != nil {
		t.Fatalf("FromJob: %v", err)
	}
	if !slices.Contains(doc.Roles, "senior_backend") {
		t.Errorf("roles = %v, want to contain senior_backend", doc.Roles)
	}
	// Named role from the title even with an empty grid.
	named, err := FromJob(db.Job{ID: 2, PublicSlug: "s2", Title: "Founding Engineer"}, skillvec.Weights{})
	if err != nil {
		t.Fatalf("FromJob: %v", err)
	}
	if !slices.Contains(named.Roles, "founding_engineer") {
		t.Errorf("roles = %v, want to contain founding_engineer", named.Roles)
	}

	// roles rides the document top level (like posted_ts) so it is filterable...
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal doc: %v", err)
	}
	var full map[string]json.RawMessage
	if err := json.Unmarshal(raw, &full); err != nil {
		t.Fatalf("unmarshal doc: %v", err)
	}
	if _, ok := full["roles"]; !ok {
		t.Errorf("document should carry a top-level roles field: %s", raw)
	}
	// ...but it must NOT be part of the public wire shape (the served jobview.Job).
	viewRaw, err := json.Marshal(doc.Job)
	if err != nil {
		t.Fatalf("marshal view: %v", err)
	}
	var view map[string]json.RawMessage
	if err := json.Unmarshal(viewRaw, &view); err != nil {
		t.Fatalf("unmarshal view: %v", err)
	}
	if _, ok := view["roles"]; ok {
		t.Errorf("roles leaked into the public job wire shape: %s", viewRaw)
	}
}

func TestFromJob_RoleTypeDerivedButIndexOnly(t *testing.T) {
	mgr, err := FromJob(db.Job{ID: 1, PublicSlug: "s", Title: "Head of Platform Engineering"}, skillvec.Weights{})
	if err != nil {
		t.Fatalf("FromJob: %v", err)
	}
	if mgr.RoleType != "people_manager" {
		t.Errorf("role_type = %q, want people_manager", mgr.RoleType)
	}

	// An unresolved title carries no value — and is still indexed. Empty means "no
	// marker in the title", never "individual contributor".
	ic, err := FromJob(db.Job{ID: 2, PublicSlug: "s2", Title: "Backend Engineer"}, skillvec.Weights{})
	if err != nil {
		t.Fatalf("FromJob: %v", err)
	}
	if ic.RoleType != "" {
		t.Errorf("role_type = %q, want empty", ic.RoleType)
	}

	// Top level on the document so it is filterable...
	raw, err := json.Marshal(mgr)
	if err != nil {
		t.Fatalf("marshal doc: %v", err)
	}
	var full map[string]json.RawMessage
	if err := json.Unmarshal(raw, &full); err != nil {
		t.Fatalf("unmarshal doc: %v", err)
	}
	if _, ok := full["role_type"]; !ok {
		t.Errorf("document should carry a top-level role_type field: %s", raw)
	}
	// ...but never part of the public wire shape.
	viewRaw, err := json.Marshal(mgr.Job)
	if err != nil {
		t.Fatalf("marshal view: %v", err)
	}
	var view map[string]json.RawMessage
	if err := json.Unmarshal(viewRaw, &view); err != nil {
		t.Fatalf("unmarshal view: %v", err)
	}
	if _, ok := view["role_type"]; ok {
		t.Errorf("role_type leaked into the public job wire shape: %s", viewRaw)
	}
}

func TestCategoryUnresolved(t *testing.T) {
	tests := []struct {
		name string
		job  db.Job
		want bool
	}{
		{"dict resolved category wins regardless of enrichment", db.Job{Category: "backend"}, false},
		{"dict empty, no enrichment at all", db.Job{}, true},
		{"dict empty, enrichment present but no category", db.Job{Enrichment: []byte(`{"summary":"x"}`)}, true},
		{"dict empty, LLM also says other", db.Job{Enrichment: []byte(`{"category":"other"}`)}, true},
		{"dict empty, LLM found a real category", db.Job{Enrichment: []byte(`{"category":"hardware"}`)}, false},
		{"dict empty, malformed enrichment JSON", db.Job{Enrichment: []byte(`not json`)}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CategoryUnresolved(tt.job); got != tt.want {
				t.Errorf("CategoryUnresolved(%+v) = %v, want %v", tt.job, got, tt.want)
			}
		})
	}
}

// A posting with no body is not a listing anyone can act on, so it stays out of the index.
// Whitespace-only counts as empty — a source that serves "<p> </p>" has published nothing,
// and sanitizing leaves the markup behind. freehire#1866.
func TestDescriptionMissing(t *testing.T) {
	tests := []struct {
		name string
		job  db.Job
		want bool
	}{
		{"a real body", db.Job{Description: "<p>Build things.</p>"}, false},
		{"empty", db.Job{}, true},
		{"whitespace only", db.Job{Description: "   \n\t "}, true},
		{"markup with no text", db.Job{Description: "<p> </p><br/>"}, true},
		{"markup wrapping text", db.Job{Description: "<div><p>Hi</p></div>"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DescriptionMissing(tt.job); got != tt.want {
				t.Errorf("DescriptionMissing(%q) = %v, want %v", tt.job.Description, got, tt.want)
			}
		})
	}
}

func TestMergeClusterGeography_WidensCanonFacets(t *testing.T) {
	doc := JobDocument{Job: jobview.Job{
		Countries: []string{"de"},
		Regions:   []string{"eu"},
		Cities:    []string{"Düsseldorf"},
	}}
	doc.MergeClusterGeography(
		[]string{"at", "de", "pl"},
		[]string{"eu"},
		[]string{"Kraków", "Wien", "Düsseldorf"},
	)
	if got, want := doc.Countries, []string{"at", "de", "pl"}; !slices.Equal(got, want) {
		t.Errorf("countries = %v, want sorted union %v", got, want)
	}
	if got, want := doc.Regions, []string{"eu"}; !slices.Equal(got, want) {
		t.Errorf("regions = %v, want %v", got, want)
	}
	if got, want := doc.Cities, []string{"Düsseldorf", "Kraków", "Wien"}; !slices.Equal(got, want) {
		t.Errorf("cities = %v, want sorted union %v", got, want)
	}
}

func TestMergeClusterGeography_EmptyClusterLeavesFacetsUnchanged(t *testing.T) {
	doc := JobDocument{Job: jobview.Job{
		Countries: []string{"de"},
		Cities:    []string{"Düsseldorf"},
	}}
	doc.MergeClusterGeography(nil, nil, nil)
	if got, want := doc.Countries, []string{"de"}; !slices.Equal(got, want) {
		t.Errorf("countries = %v, want unchanged %v", got, want)
	}
	if got, want := doc.Cities, []string{"Düsseldorf"}; !slices.Equal(got, want) {
		t.Errorf("cities = %v, want unchanged %v", got, want)
	}
}

func TestFromJob_AIArchetypeDerivedButIndexOnly(t *testing.T) {
	doc, err := FromJob(db.Job{
		ID: 1, PublicSlug: "s", Category: "ai_engineering",
		Skills: []string{"rag", "langchain", "langgraph", "vector-databases"},
	}, skillvec.Weights{})
	if err != nil {
		t.Fatalf("FromJob: %v", err)
	}
	if doc.AIArchetype != "rag_app_builder" {
		t.Errorf("ai_archetype = %q, want rag_app_builder", doc.AIArchetype)
	}

	// A non-AI category yields no archetype, regardless of skills.
	nonAI, err := FromJob(db.Job{
		ID: 2, PublicSlug: "s2", Category: "backend",
		Skills: []string{"rag", "langchain", "langgraph", "vector-databases"},
	}, skillvec.Weights{})
	if err != nil {
		t.Fatalf("FromJob: %v", err)
	}
	if nonAI.AIArchetype != "" {
		t.Errorf("ai_archetype = %q, want empty for category backend", nonAI.AIArchetype)
	}

	// ai_archetype rides the document top level (like roles) so it is filterable...
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal doc: %v", err)
	}
	var full map[string]json.RawMessage
	if err := json.Unmarshal(raw, &full); err != nil {
		t.Fatalf("unmarshal doc: %v", err)
	}
	if _, ok := full["ai_archetype"]; !ok {
		t.Errorf("document should carry a top-level ai_archetype field: %s", raw)
	}
	// ...but it must NOT be part of the public wire shape (the served jobview.Job).
	viewRaw, err := json.Marshal(doc.Job)
	if err != nil {
		t.Fatalf("marshal view: %v", err)
	}
	var view map[string]json.RawMessage
	if err := json.Unmarshal(viewRaw, &view); err != nil {
		t.Fatalf("unmarshal view: %v", err)
	}
	if _, ok := view["ai_archetype"]; ok {
		t.Errorf("ai_archetype leaked into the public job wire shape: %s", viewRaw)
	}
}

func TestFromJob_DocumentFlattensIDAndViewToTopLevelJSON(t *testing.T) {
	// Meilisearch reads the primary key "id" from the top level of the document,
	// and the embedded jobview.Job must flatten (no nesting) so its fields are
	// the searchable attributes. A json tag on the embedded field would break
	// this. Enrichment itself stays a nested object (filtered via dot paths).
	doc, err := FromJob(db.Job{ID: 42, Title: "Go Dev", PublicSlug: "go-dev-acme-x"}, skillvec.Weights{})
	if err != nil {
		t.Fatalf("FromJob: %v", err)
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := string(m["id"]); got != "42" {
		t.Errorf("top-level id = %s, want 42 in %s", got, raw)
	}
	if got := string(m["public_slug"]); got != `"go-dev-acme-x"` {
		t.Errorf("public_slug not flattened to top level: %s", raw)
	}
	if _, ok := m["enrichment"]; !ok {
		t.Errorf("enrichment should be a nested object in %s", raw)
	}
}

func TestFromJob_PostedTSIsEffectiveDateEpoch(t *testing.T) {
	// posted_ts is the numeric (unix-seconds) encoding of the SAME effective posting
	// date the display posted_at reflects, so the Meilisearch range filter agrees with
	// what the user sees. A past posted_at is used directly; a future one falls back to
	// created_at (mirroring jobview.EffectivePostedAt).
	created := pgtype.Timestamptz{Time: time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC), Valid: true}
	past := pgtype.Timestamptz{Time: time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC), Valid: true}

	doc, err := FromJob(db.Job{ID: 1, PublicSlug: "s", CreatedAt: created, PostedAt: past}, skillvec.Weights{})
	if err != nil {
		t.Fatalf("FromJob: %v", err)
	}
	if doc.PostedTS != past.Time.Unix() {
		t.Errorf("posted_ts = %d, want %d (past posted_at)", doc.PostedTS, past.Time.Unix())
	}

	future := pgtype.Timestamptz{Time: time.Now().Add(48 * time.Hour), Valid: true}
	docFuture, err := FromJob(db.Job{ID: 2, PublicSlug: "s2", CreatedAt: created, PostedAt: future}, skillvec.Weights{})
	if err != nil {
		t.Fatalf("FromJob future: %v", err)
	}
	if docFuture.PostedTS != created.Time.Unix() {
		t.Errorf("posted_ts = %d, want %d (future posted_at falls back to created_at)", docFuture.PostedTS, created.Time.Unix())
	}

	// posted_ts is an index-only field: it must be present on the document JSON (so
	// Meilisearch can filter on it) but absent from the public job wire shape served
	// to clients (jobview.Job).
	rawDoc, _ := json.Marshal(doc)
	var docMap map[string]json.RawMessage
	if err := json.Unmarshal(rawDoc, &docMap); err != nil {
		t.Fatalf("unmarshal doc: %v", err)
	}
	if _, ok := docMap["posted_ts"]; !ok {
		t.Errorf("posted_ts missing from index document: %s", rawDoc)
	}
	rawView, _ := json.Marshal(jobview.Job{PublicSlug: "s"})
	var viewMap map[string]json.RawMessage
	if err := json.Unmarshal(rawView, &viewMap); err != nil {
		t.Fatalf("unmarshal view: %v", err)
	}
	if _, ok := viewMap["posted_ts"]; ok {
		t.Errorf("posted_ts leaked into the public job wire shape: %s", rawView)
	}
}

func TestFromJob_CapsIndexedDescription(t *testing.T) {
	// The search document caps the description so the inverted index (and a full
	// rebuild's transient disk footprint) stays small; the detail endpoint still
	// serves the full text from Postgres. A long description is trimmed to at most
	// maxIndexedDescriptionRunes runes; a short one is left verbatim.
	long := strings.Repeat("alpha beta ", 600) // ~6000 runes
	doc, err := FromJob(db.Job{ID: 1, Title: "Go Dev", Description: long}, skillvec.Weights{})
	if err != nil {
		t.Fatalf("FromJob: %v", err)
	}
	if n := utf8.RuneCountInString(doc.Description); n > maxIndexedDescriptionRunes {
		t.Errorf("indexed description = %d runes, want <= %d", n, maxIndexedDescriptionRunes)
	}
	if !strings.HasPrefix(long, doc.Description) {
		t.Errorf("truncated description is not a prefix of the original")
	}

	short := "Build backend services in Go."
	docShort, err := FromJob(db.Job{ID: 2, Title: "Go Dev", Description: short}, skillvec.Weights{})
	if err != nil {
		t.Fatalf("FromJob short: %v", err)
	}
	if docShort.Description != short {
		t.Errorf("short description altered: got %q, want %q", docShort.Description, short)
	}
}
